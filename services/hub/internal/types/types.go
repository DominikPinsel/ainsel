package types

import (
	"encoding/json"
	"time"
)

// ErrorEntry represents a platform error event.
type ErrorEntry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Severity  string                 `json:"severity"`
	Source    string                 `json:"source"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ActivityEntry represents one event flowing through the platform.
type ActivityEntry struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Connector string          `json:"connector"`
	Status    string          `json:"status"`
	Matches   []MatchResult   `json:"matches,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// MatchResult records which trigger matched and which agent received the event.
type MatchResult struct {
	Trigger string `json:"trigger"`
	Agent   string `json:"agent"`
}
