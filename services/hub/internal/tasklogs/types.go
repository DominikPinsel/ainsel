// Package tasklogs provides storage and retrieval of structured log entries
// published by agents during task execution. Entries are persisted in
// Postgres so the hub's observability endpoints work without an external
// log backend required.
package tasklogs

import "time"

// Level values for a log entry.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Entry is a single structured log line published by an agent.
type Entry struct {
	ID            int64          `json:"id"`
	InvocationID  string         `json:"invocationId,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
	AgentName     string         `json:"agentName"`
	Level         string         `json:"level"`
	Message       string         `json:"message"`
	Fields        map[string]any `json:"fields,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
}

// ListOptions filters and paginates List results.
type ListOptions struct {
	AgentName string
	Level     string
	Since     time.Time
	Until     time.Time
	Limit     int
}

// ConversationMessage is a single message in an agent conversation,
// captured from the pi RPC event stream (assistant responses).
type ConversationMessage struct {
	ID            int64     `json:"id"`
	InvocationID  string    `json:"invocationId,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	AgentName     string    `json:"agentName"`
	Role          string    `json:"role"`
	Content       string    `json:"content"`
	Model         string    `json:"model,omitempty"`
	InputTokens   int       `json:"inputTokens,omitempty"`
	OutputTokens  int       `json:"outputTokens,omitempty"`
	StopReason    string    `json:"stopReason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}
