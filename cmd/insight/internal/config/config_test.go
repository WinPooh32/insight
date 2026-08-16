package config_test

import (
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/config"
)

func TestParseEventFilter(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     []string
	}{
		{
			name:     "empty means allow all",
			envValue: "",
			want:     nil,
		},
		{
			name:     "single event",
			envValue: "SessionStart",
			want:     []string{"SessionStart"},
		},
		{
			name:     "multiple events",
			envValue: "SessionStart,UserPromptSubmit,Stop",
			want:     []string{"SessionStart", "UserPromptSubmit", "Stop"},
		},
		{
			name:     "trims whitespace",
			envValue: "SessionStart , UserPromptSubmit , Stop",
			want:     []string{"SessionStart", "UserPromptSubmit", "Stop"},
		},
		{
			name:     "deduplicates",
			envValue: "SessionStart,SessionStart,Stop",
			want:     []string{"SessionStart", "Stop"},
		},
		{
			name:     "skips empty elements",
			envValue: "SessionStart,,Stop",
			want:     []string{"SessionStart", "Stop"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INSIGHT_EVENT_FILTER", tc.envValue)

			cfg := config.DefaultConfig()

			got := cfg.EventFilter
			if len(got) != len(tc.want) {
				t.Errorf("EventFilter length = %d, want %d (got %v, want %v)", len(got), len(tc.want), got, tc.want)
			}

			for i := range tc.want {
				if i < len(got) && got[i] != tc.want[i] {
					t.Errorf("EventFilter[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	t.Setenv("EMBED_BASE_URL", "")
	t.Setenv("EMBED_MODEL", "")
	t.Setenv("EMBED_API_KEY", "")

	cfg := config.DefaultConfig()

	if cfg.Port != 8765 {
		t.Errorf("default Port = %d, want 8765", cfg.Port)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("default Host = %q, want 127.0.0.1", cfg.Host)
	}

	if cfg.EmbedBaseURL != "http://localhost:8080/v1" {
		t.Errorf("default EmbedBaseURL = %q, want http://localhost:8080/v1", cfg.EmbedBaseURL)
	}

	if cfg.EmbedModel != "" {
		t.Errorf("default EmbedModel = %q, want empty", cfg.EmbedModel)
	}

	if cfg.EmbedAPIKey != "" {
		t.Errorf("default EmbedAPIKey = %q, want empty", cfg.EmbedAPIKey)
	}
}

func TestConfig_EmbedEnv(t *testing.T) {
	t.Setenv("EMBED_BASE_URL", "https://api.example.com/v1")
	t.Setenv("EMBED_MODEL", "model-x")
	t.Setenv("EMBED_API_KEY", "key-123")

	cfg := config.DefaultConfig()

	if cfg.EmbedBaseURL != "https://api.example.com/v1" {
		t.Errorf("EmbedBaseURL = %q, want https://api.example.com/v1", cfg.EmbedBaseURL)
	}

	if cfg.EmbedModel != "model-x" {
		t.Errorf("EmbedModel = %q, want model-x", cfg.EmbedModel)
	}

	if cfg.EmbedAPIKey != "key-123" {
		t.Errorf("EmbedAPIKey = %q, want key-123", cfg.EmbedAPIKey)
	}
}
