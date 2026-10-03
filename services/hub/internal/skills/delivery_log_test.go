package skills

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// runDeliveryLog calls logDeliveryChanges with the default logger pointed at
// a buffer, and returns what it logged. The delivery loop runs every 30 s
// while something is undelivered, so what this captures is the log volume an
// operator actually sees across passes — which is the whole point of the
// gating, and the reason it is tested here rather than through Service.
// Converge, which needs a registry and a cluster.
func runDeliveryLog(t *testing.T, s *Service, delivered []string, undelivered map[string]error, enabled int) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	s.logDeliveryChanges(delivered, undelivered, enabled)
	return buf.String()
}

func countLines(out, substr string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

func TestLogDeliveryChangesRepeatsNothingUnchanged(t *testing.T) {
	s := &Service{}
	tooLong := errors.New(`update configmap skills: ConfigMap "skills" is invalid: []: Too long: may not be more than 1048576 bytes`)

	first := runDeliveryLog(t, s, nil, map[string]error{"a": tooLong, "b": tooLong}, 2)
	if got := countLines(first, "level=WARN"); got != 2 {
		t.Fatalf("first pass logged %d WARN lines, want one per undelivered skill:\n%s", got, first)
	}
	for _, want := range []string{`skill_id=a`, `skill_id=b`, "1048576 bytes"} {
		if !strings.Contains(first, want) {
			t.Errorf("first pass log is missing %q:\n%s", want, first)
		}
	}

	// The same failure on the next pass must be silent: the retry cadence
	// is 30 s, so repeating it would drown the line that reports a new
	// failure.
	if again := runDeliveryLog(t, s, nil, map[string]error{"a": tooLong, "b": tooLong}, 2); strings.TrimSpace(again) != "" {
		t.Errorf("an unchanged undelivered set must not log again, got:\n%s", again)
	}

	// A new skill failing is still reported, and reported alone.
	third := runDeliveryLog(t, s, nil, map[string]error{"a": tooLong, "b": tooLong, "c": errors.New("get configmap skills: forbidden")}, 3)
	if got := countLines(third, "level=WARN"); got != 1 || !strings.Contains(third, `skill_id=c`) {
		t.Errorf("want exactly one WARN for the new skill c, got %d:\n%s", got, third)
	}

	// A different reason for a skill already reported is a change.
	shrank := runDeliveryLog(t, s, nil, map[string]error{"a": errors.New("configmap not found")}, 1)
	if !strings.Contains(shrank, `skill_id=a`) {
		t.Errorf("a changed reason must be reported again:\n%s", shrank)
	}
}

func TestLogDeliveryChangesReportsRecoveryOnlyWhenDelivered(t *testing.T) {
	s := &Service{}
	failed := map[string]error{"a": errors.New("too long"), "b": errors.New("too long")}
	if out := runDeliveryLog(t, s, nil, failed, 2); !strings.Contains(out, `skill_id=b`) {
		t.Fatalf("setup: both skills should be reported:\n%s", out)
	}

	// b is delivered on the retry; a is still missing. Only b gets a line,
	// and it is INFO so it does not read as a fresh failure.
	out := runDeliveryLog(t, s, []string{"b"}, map[string]error{"a": errors.New("too long")}, 2)
	if got := countLines(out, "level=INFO"); got != 1 || !strings.Contains(out, `skill_id=b`) {
		t.Errorf("want one INFO recovery line for b, got %d:\n%s", got, out)
	}
	if strings.Contains(out, `skill_id=a`) {
		t.Errorf("still-undelivered skill must not re-WARN:\n%s", out)
	}

	// Nothing new to say once the state has been reported.
	if again := runDeliveryLog(t, s, []string{"b"}, map[string]error{"a": errors.New("too long")}, 2); strings.TrimSpace(again) != "" {
		t.Errorf("second identical pass must be silent, got:\n%s", again)
	}

	// b stops being enabled entirely (an agent un-assigned it). It was never
	// delivered, so claiming a recovery would be a lie: forget it silently.
	if out := runDeliveryLog(t, s, nil, map[string]error{"a": errors.New("too long")}, 1); strings.TrimSpace(out) != "" {
		t.Errorf("a skill that left the enabled set must not log, got:\n%s", out)
	}

	// ...and if it is enabled again and fails again, that is a new failure.
	out = runDeliveryLog(t, s, nil, map[string]error{"a": errors.New("too long"), "b": errors.New("too long")}, 2)
	if got := countLines(out, "level=WARN"); got != 1 || !strings.Contains(out, `skill_id=b`) {
		t.Errorf("want a fresh WARN for b only, got %d:\n%s", got, out)
	}
}
