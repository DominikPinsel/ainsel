// Package cron parses 5-field cron expressions and computes the next fire
// time. It is a self-contained replacement for a third-party cron library so
// the hub (scheduler) and the agent operator (schedule validation) can share
// one parser without adding a dependency.
//
// Supported expression syntax (standard 5-field cron, minute precision):
//
//	field          := <minute> <hour> <day-of-month> <month> <day-of-week>
//	value          := "*" | number | range | step | list
//	range         := low "-" high
//	step          := value "/" step
//	list          := value ("," value)*
//
// Day-of-week uses 0-7 where both 0 and 7 mean Sunday. Names (sun, mon, ...) are
// not supported — use numbers. As in Vixie cron, when both day-of-month and
// day-of-week are restricted (not "*"), a match on either is sufficient.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed cron expression that can compute the next fire time
// after a given instant.
type Schedule struct {
	minute, hour, dom, month, dow *bitmask
}

// bitmask is a 64-bit set over the valid range of a field (0-59 for minutes,
// 0-23 for hours, 1-31 for dom, 1-12 for month, 0-7 for dow). A nil bitmask
// means "any value" (i.e. the field is "*").
type bitmask uint64

func (b bitmask) has(v int) bool { return b&(bitmask(1)<<v) != 0 }

// Parse parses a 5-field cron expression and returns a Schedule.
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron: minute field %q: %w", fields[0], err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron: hour field %q: %w", fields[1], err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-month field %q: %w", fields[2], err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron: month field %q: %w", fields[3], err)
	}
	dow, err := parseField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-week field %q: %w", fields[4], err)
	}
	// Normalize 7 -> 0 so Sunday has a single canonical bit.
	if dow != nil && dow.has(7) {
		*dow |= bitmask(1) << 0
		*dow &^= bitmask(1) << 7
	}

	return &Schedule{
		minute: minute,
		hour:   hour,
		dom:    dom,
		month:  month,
		dow:    dow,
	}, nil
}

// parseField parses one cron field into a bitmask over [min,max]. A nil result
// (with no error) means the field is "*" — match any value.
func parseField(field string, min, max int) (*bitmask, error) {
	if field == "*" {
		return nil, nil
	}

	bits := bitmask(0)
	for _, part := range strings.Split(field, ",") {
		b, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		bits |= b
	}
	return &bits, nil
}

// parsePart parses a single comma-separated entry, which may be a number,
// a range (low-high), a step (value/step), or a range with step.
func parsePart(part string, min, max int) (bitmask, error) {
	step := 1
	rangePart := part

	if idx := strings.Index(part, "/"); idx >= 0 {
		var err error
		step, err = strconv.Atoi(part[idx+1:])
		if err != nil || step <= 0 {
			return 0, fmt.Errorf("invalid step %q", part[idx+1:])
		}
		rangePart = part[:idx]
	}

	var low, high int
	if rangePart == "*" {
		low, high = min, max
	} else if idx := strings.Index(rangePart, "-"); idx >= 0 {
		lo, err1 := strconv.Atoi(rangePart[:idx])
		hi, err2 := strconv.Atoi(rangePart[idx+1:])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid range %q", rangePart)
		}
		low, high = lo, hi
	} else {
		n, err := strconv.Atoi(rangePart)
		if err != nil {
			return 0, fmt.Errorf("invalid value %q", rangePart)
		}
		low, high = n, n
	}

	if low < min || high > max || low > high {
		return 0, fmt.Errorf("value %d-%d out of range [%d,%d]", low, high, min, max)
	}

	var bits bitmask
	for v := low; v <= high; v += step {
		bits |= bitmask(1) << v
	}
	return bits, nil
}

// Next returns the next instant strictly after t at which the schedule fires.
// It returns the zero time if no fire occurs within approximately five years
// (a safety bound to prevent runaway loops).
func (s *Schedule) Next(t time.Time) time.Time {
	// Cron has minute precision; align to the start of the next minute.
	loc := t.Location()
	origin := t.Truncate(time.Minute).Add(time.Minute)

	limit := origin.AddDate(5, 0, 0)

	for cur := origin; cur.Before(limit); cur = cur.AddDate(0, 0, 1) {
		if s.month != nil && !s.month.has(int(cur.Month())) {
			continue
		}
		// Day match follows Vixie cron semantics:
		//  - If both day-of-month and day-of-week are restricted (neither is "*"),
		//    a match on EITHER is sufficient.
		//  - Otherwise both must match (an unrestricted field always matches).
		domStar := s.dom == nil
		dowStar := s.dow == nil
		domHit := domStar || s.dom.has(cur.Day())
		dowHit := dowStar || s.dow.has(int(cur.Weekday()))
		var dayMatch bool
		if domStar || dowStar {
			dayMatch = domHit && dowHit
		} else {
			dayMatch = domHit || dowHit
		}
		if !dayMatch {
			continue
		}

		// Found a matching day; scan its hours and minutes.
		for h := 0; h < 24; h++ {
			if s.hour != nil && !s.hour.has(h) {
				continue
			}
			for m := 0; m < 60; m++ {
				if s.minute != nil && !s.minute.has(m) {
					continue
				}
				cand := time.Date(cur.Year(), cur.Month(), cur.Day(), h, m, 0, 0, loc)
				if cand.After(t) {
					return cand
				}
			}
		}
	}
	return time.Time{}
}

// describe is a small helper used only in tests to render a schedule's bit set
// for a single field as a list of values.
func describe(b *bitmask, max int) []int {
	if b == nil {
		out := make([]int, max+1)
		for i := range out {
			out[i] = i
		}
		return out
	}
	var out []int
	for v := 0; v <= max; v++ {
		if b.has(v) {
			out = append(out, v)
		}
	}
	return out
}
