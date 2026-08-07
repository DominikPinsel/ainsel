package db

import "testing"

func TestEnvInt(t *testing.T) {
	tests := []struct {
		name string
		env  string // value to set; empty string means unset
		set  bool
		want int32
	}{
		{name: "unset falls back to default", set: false, want: 7},
		{name: "empty falls back to default", env: "", set: true, want: 7},
		{name: "valid positive value", env: "30", set: true, want: 30},
		{name: "one is allowed", env: "1", set: true, want: 1},
		{name: "zero falls back to default", env: "0", set: true, want: 7},
		{name: "negative falls back to default", env: "-3", set: true, want: 7},
		{name: "garbage falls back to default", env: "ten", set: true, want: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(EnvMaxConns, tt.env)
			}
			if got := envInt(EnvMaxConns, 7); got != tt.want {
				t.Fatalf("envInt(%q) = %d, want %d", tt.env, got, tt.want)
			}
		})
	}
}
