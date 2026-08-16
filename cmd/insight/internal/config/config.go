package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the relay service configuration.
type Config struct {
	Port        int
	Host        string
	DataDir     string
	EventFilter []string // allowed event types; empty = allow all
	Logger      *slog.Logger

	EmbedBaseURL string // OpenAI-compatible embeddings API base, e.g. http://localhost:8080/v1
	EmbedModel   string
	EmbedAPIKey  string // empty for local serving
}

// DefaultConfig loads configuration from environment variables.
func DefaultConfig() Config {
	port := 8765

	if p := os.Getenv("INSIGHT_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	host := os.Getenv("INSIGHT_HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	dataDir := os.Getenv("INSIGHT_DATA_DIR")
	if dataDir == "" {
		home := os.Getenv("HOME")
		dataDir = filepath.Join(home, ".insight")
	}

	level := &slog.LevelVar{}
	if err := level.UnmarshalText([]byte(os.Getenv("INSIGHT_LOG_LEVEL"))); err != nil {
		level.Set(slog.LevelInfo)
	}

	opts := slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: nil,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &opts))

	eventFilter := parseEventFilter()

	embedBaseURL := os.Getenv("EMBED_BASE_URL")
	if embedBaseURL == "" {
		embedBaseURL = "http://localhost:8080/v1"
	}

	embedModel := os.Getenv("EMBED_MODEL")
	embedAPIKey := os.Getenv("EMBED_API_KEY")

	return Config{
		Port:         port,
		Host:         host,
		DataDir:      dataDir,
		EventFilter:  eventFilter,
		Logger:       logger,
		EmbedBaseURL: embedBaseURL,
		EmbedModel:   embedModel,
		EmbedAPIKey:  embedAPIKey,
	}
}

// parseEventFilter reads INSIGHT_EVENT_FILTER, splits on comma, trims whitespace,
// and deduplicates. Empty string returns nil (meaning "allow all").
func parseEventFilter() []string {
	raw := os.Getenv("INSIGHT_EVENT_FILTER")
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))

	var result []string

	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}

		if _, ok := seen[trimmed]; ok {
			continue
		}

		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// Addr returns the listen address for the HTTP server.
func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// DBPath returns the full path to the SQLite database file.
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "events.db")
}
