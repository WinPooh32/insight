package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds the relay service configuration.
type Config struct {
	Port    int
	Host    string
	DataDir string
	Logger  *slog.Logger

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
		Logger:       logger,
		EmbedBaseURL: embedBaseURL,
		EmbedModel:   embedModel,
		EmbedAPIKey:  embedAPIKey,
	}
}

// Addr returns the listen address for the HTTP server.
func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// DBPath returns the full path to the SQLite database file.
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "events.db")
}
