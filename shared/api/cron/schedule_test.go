package cron

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", expr, err)
	}
	return s
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",              // too few fields
		"* * *",         // too few fields
		"* * * * * *",   // too many fields
		"60 * * * *",    // minute out of range
		"* 24 * * *",    // hour out of range
		"* * 0 * *",     // dom out of range (min 1)
		"* * * 13 *",    // month out of range
		"* * * * 8",     // dow out of range (0-7)
		"abc * * * *",   // non-numeric
		"1-5/0 * * * *", // step zero
		"10-5 * * * *",  // range backwards
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", c)
		}
	}
}

func TestStarMatchesAny(t *testing.T) {
	s := mustParse(t, "* * * * *")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next := s.Next(start)
	want := start.Add(time.Minute)
	if !next.Equal(want) {
		t.Errorf("Next(* * * * *) = %v, want %v", next, want)
	}
}

func TestEveryFiveMinutes(t *testing.T) {
	s := mustParse(t, "*/5 * * * *")
	start := time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC)
	next := s.Next(start)
	want := time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next(*/5) = %v, want %v", next, want)
	}
}

func TestWeekdaySchedule(t *testing.T) {
	// 09:00 on weekdays (Mon-Fri).
	s := mustParse(t, "0 9 * * 1-5")
	// 2026-01-02 is a Friday.
	fri := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	next := s.Next(fri)
	want := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC) // next Monday 09:00
	if !next.Equal(want) {
		t.Errorf("Next(0 9 * * 1-5) from Fri 10:00 = %v, want %v", next, want)
	}
}

func TestSundayNormalization(t *testing.T) {
	// 0 and 7 both mean Sunday; "0 0 * * 7" should match the same Sundays as "0 0 * * 0".
	s7 := mustParse(t, "0 0 * * 7")
	s0 := mustParse(t, "0 0 * * 0")
	start := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC) // Saturday
	n7 := s7.Next(start)
	n0 := s0.Next(start)
	if !n7.Equal(n0) {
		t.Errorf("Sunday via 7 (%v) != Sunday via 0 (%v)", n7, n0)
	}
	want := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC) // Sunday midnight
	if !n7.Equal(want) {
		t.Errorf("Next(0 0 * * 7) = %v, want %v", n7, want)
	}
}

func TestDomOrDow(t *testing.T) {
	// Vixie cron: when both dom and dow are restricted, either matching suffices.
	// "0 0 1 * 1" fires on the 1st of the month OR any Monday.
	s := mustParse(t, "0 0 1 * 1")
	start := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC) // Saturday
	next := s.Next(start)
	want := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday Jan 5
	if !next.Equal(want) {
		t.Errorf("Next(0 0 1 * 1) = %v, want %v", next, want)
	}
}

func TestListField(t *testing.T) {
	s := mustParse(t, "0,30 * * * *")
	start := time.Date(2026, 1, 1, 0, 15, 0, 0, time.UTC)
	next := s.Next(start)
	want := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next(0,30) = %v, want %v", next, want)
	}
}

func TestNextAtBoundary(t *testing.T) {
	// A fire time exactly at the input is in the past (at-or-after is strict),
	// so the next fire is the following day.
	s := mustParse(t, "0 0 * * *")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // midnight exactly
	next := s.Next(start)
	want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next at boundary = %v, want %v", next, want)
	}
}

func TestDescribeAnyField(t *testing.T) {
	got := describe(nil, 4)
	want := []int{0, 1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("describe(any) len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("describe(any)[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}
