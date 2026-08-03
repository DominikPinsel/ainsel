package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateTagsNormalizes(t *testing.T) {
	got, err := validateTags([]string{"  Go ", "REVIEW", "security"})
	if err != nil {
		t.Fatalf("validateTags: %v", err)
	}
	want := []string{"go", "review", "security"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tag[%d]: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestValidateTagsDropsEmptyEntries(t *testing.T) {
	got, err := validateTags([]string{"", "   ", "go", "\t"})
	if err != nil {
		t.Fatalf("validateTags: %v", err)
	}
	if len(got) != 1 || got[0] != "go" {
		t.Errorf("expected [go], got %v", got)
	}
}

func TestValidateTagsDeduplicates(t *testing.T) {
	// Duplicates that differ only by case/whitespace collapse to one entry.
	got, err := validateTags([]string{"go", "Go", " GO ", "review"})
	if err != nil {
		t.Fatalf("validateTags: %v", err)
	}
	if len(got) != 2 || got[0] != "go" || got[1] != "review" {
		t.Errorf("expected [go review], got %v", got)
	}
}

func TestValidateTagsCountAfterNormalization(t *testing.T) {
	// 11 empty strings normalize to zero tags and must NOT be rejected,
	// even though the raw input length exceeds maxTags.
	empties := make([]string, maxTags+1)
	got, err := validateTags(empties)
	if err != nil {
		t.Fatalf("expected no error for empty entries, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero tags, got %v", got)
	}
}

func TestValidateTagsRejectsTooMany(t *testing.T) {
	tags := make([]string, maxTags+1)
	for i := range tags {
		tags[i] = "tag" + string(rune('a'+i))
	}
	_, err := validateTags(tags)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if verr.Field != "tags" {
		t.Errorf("expected field tags, got %q", verr.Field)
	}
}

func TestValidateTagsRejectsTooLong(t *testing.T) {
	_, err := validateTags([]string{strings.Repeat("x", maxTagLen+1)})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if verr.Field != "tags" {
		t.Errorf("expected field tags, got %q", verr.Field)
	}
}

func TestEscapeILIKE(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
		{"50%_off\\sale", `50\%\_off\\sale`},
	}
	for _, c := range cases {
		if got := escapeILIKE(c.in); got != c.want {
			t.Errorf("escapeILIKE(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
