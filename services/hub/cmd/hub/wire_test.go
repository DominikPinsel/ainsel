package main

import (
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/api"
)

func TestWireMetricsSource(t *testing.T) {
	// The chart value and the response field are meant to read the same, so the
	// accepted tokens are the api package's, not a second spelling invented here.
	// An unrecognised value must land on the default rather than fail startup:
	// this setting chooses between two backends that both work, so a typo costs a
	// log line, not a pod.
	cases := []struct {
		raw, want string
	}{
		{"", api.MetricsSourceRecords},
		{"postgres", api.MetricsSourceRecords},
		{"POSTGRES", api.MetricsSourceRecords},
		{"  postgres  ", api.MetricsSourceRecords},
		{"prometheus", api.MetricsSourcePrometheus},
		{"Prometheus", api.MetricsSourcePrometheus},
		{"prometheus ", api.MetricsSourcePrometheus},
		{"victoriametrics", api.MetricsSourceRecords},
		{"prom", api.MetricsSourceRecords},
		{"records", api.MetricsSourceRecords},
	}
	for _, tc := range cases {
		if got := wireMetricsSource(tc.raw); got != tc.want {
			t.Errorf("wireMetricsSource(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
