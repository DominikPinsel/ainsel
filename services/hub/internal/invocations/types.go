// Package invocations provides storage and retrieval of agent invocation
// history. An "invocation" represents a single dispatch of an event to a
// matched agent — recording who was invoked, with what trigger, when it
// started, when it completed, and what its outcome was.
package invocations

import "time"

// Status values for an invocation lifecycle.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailure = "failure"
	StatusTimeout = "timeout"
)

// Invocation is a single record of an event dispatched to an agent.
//
// An invocation is created in StatusRunning when the router publishes an event
// to an agent's task queue, and is updated to a terminal status (success,
// failure, or timeout) when the agent reports completion via
// hub.invocation.completed.
type Invocation struct {
	// ID is the unique identifier for this invocation. Format: "inv-<8 hex>".
	ID string `json:"id"`

	// AgentName is the name of the agent the event was dispatched to.
	AgentName string `json:"agentName"`

	// TriggerName is the name of the trigger that matched.
	TriggerName string `json:"triggerName"`

	// EventID is the canonical ID of the originating event.
	EventID string `json:"eventId,omitempty"`

	// Connector is the connector that produced the event.
	Connector string `json:"connector,omitempty"`

	// StartTime is when the invocation was dispatched.
	StartTime time.Time `json:"startTime"`

	// EndTime is when the invocation completed. Nil while running.
	EndTime *time.Time `json:"endTime,omitempty"`

	// DurationMs is the duration in milliseconds. Nil while running.
	DurationMs *int64 `json:"durationMs,omitempty"`

	// Status is one of: running, success, failure, timeout.
	Status string `json:"status"`

	// Error is the error message when Status is failure or timeout. Empty otherwise.
	Error string `json:"error,omitempty"`
}

// IsTerminal returns true if the invocation has reached a final status.
func (i *Invocation) IsTerminal() bool {
	return i.Status == StatusSuccess || i.Status == StatusFailure || i.Status == StatusTimeout
}
