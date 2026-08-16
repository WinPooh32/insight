package config_test

import (
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/config"
)

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
