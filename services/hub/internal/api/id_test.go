package api

import (
	"regexp"
	"testing"
)

func TestGenerateID(t *testing.T) {
	tests := []struct {
		prefix string
		want   *regexp.Regexp
	}{
		{"c", regexp.MustCompile(`^c-[0-9a-f]{8}$`)},
		{"a", regexp.MustCompile(`^a-[0-9a-f]{8}$`)},
		{"t", regexp.MustCompile(`^t-[0-9a-f]{8}$`)},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			id := generateID(tt.prefix)
			if !tt.want.MatchString(id) {
				t.Errorf("generateID(%q) = %q, want match %s", tt.prefix, id, tt.want)
			}
		})
	}
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID("c")
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}
