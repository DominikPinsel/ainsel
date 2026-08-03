package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("HUB_URL")
	_ = os.Unsetenv("AGENT_NAME")
	_ = os.Unsetenv("HUB_INTERNAL_VALIDATE_SECRET")

	cfg := Load()
	if cfg.Port != 8081 {
		t.Fatalf("expected default port 8081, got %d", cfg.Port)
	}
	if cfg.HubURL != "" {
		t.Fatalf("expected empty default hub URL (operator injects HUB_URL), got %s", cfg.HubURL)
	}
	if cfg.AgentName != "" {
		t.Fatalf("expected empty agent name, got %s", cfg.AgentName)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9091")
	t.Setenv("HUB_URL", "http://localhost:8080")
	t.Setenv("AGENT_NAME", "developer")

	cfg := Load()
	if cfg.Port != 9091 {
		t.Fatalf("expected port 9091, got %d", cfg.Port)
	}
	if cfg.HubURL != "http://localhost:8080" {
		t.Fatalf("unexpected hub URL: %s", cfg.HubURL)
	}
	if cfg.AgentName != "developer" {
		t.Fatalf("expected agent name developer, got %s", cfg.AgentName)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: Config{
				HubURL:    "http://hub:8080",
				AgentName: "developer",
			},
			wantErr: false,
		},
		{
			name: "missing hub url",
			cfg: Config{
				AgentName: "developer",
			},
			wantErr: true,
		},
		{
			name: "missing agent name",
			cfg: Config{
				HubURL: "http://hub:8080",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}