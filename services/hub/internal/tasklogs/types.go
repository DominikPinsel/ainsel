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
	// AgentNames restricts results to any of these agents. Used to scope the
	// log view to the agents a caller may read; an empty slice applies no
	// restriction, so callers that mean "no agents at all" must not query.
	AgentNames []string
	Level      string
	Since      time.Time
	Until      time.Time
	Limit      int
}

// ConversationListOptions filters conversation message queries.
type ConversationListOptions struct {
	// AgentName restricts results to a single agent.
	AgentName string
	// AgentNames restricts results to any of these agents, for scoping to the
	// agents a caller may read. Empty applies no restriction.
	AgentNames    []string
	InvocationID  string
	CorrelationID string
	// Limit caps the number of messages returned. <= 0 means the default.
	Limit int
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
