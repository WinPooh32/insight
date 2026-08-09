package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Envelope represents a hook event received from Claude Code.
type Envelope struct {
	ID        string
	EventType string
	Received  time.Time
	Payload   map[string]any
	SessionID string
}

// NewEnvelope creates a new envelope from raw JSON payload.
func NewEnvelope(eventType string, payload map[string]any) Envelope {
	// Extract session_id if present
	var sessionID string
	if sid, ok := payload["session_id"].(string); ok {
		sessionID = sid
	}

	return Envelope{
		ID:        uuid.New().String(),
		EventType: eventType,
		Received:  time.Now().UTC(),
		Payload:   payload,
		SessionID: sessionID,
	}
}

// ToJSON serializes the payload to JSON bytes for storage.
func (e Envelope) ToJSON() ([]byte, error) {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	return payload, nil
}

// StoredEvent represents an event as persisted in storage.
// This type lives in the domain layer so the repository interface
// does not leak infrastructure types (sqlc models) into presentation layers.
type StoredEvent struct {
	ID        string  `json:"id"`
	EventType string  `json:"event_type"`
	Received  string  `json:"received"`
	Payload   string  `json:"payload"`
	SessionID *string `json:"session_id,omitempty"`
}
