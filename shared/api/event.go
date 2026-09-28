// Package common defines shared types for the Ainsel agent platform.
package ainselapishared

import (
	"encoding/json"
	"time"
)

// RawJSON is an alias for json.RawMessage for event data payloads.
type RawJSON = json.RawMessage

// Event is the canonical event format produced by connectors and consumed by the hub.
// Type, Subject, Actor, and Action are removed — raw payload + headers carry all information.
type Event struct {
	ID        string            `json:"id"`
	Version   string            `json:"version"`
	Connector string            `json:"connector"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers,omitempty"`
	Data      RawJSON           `json:"data,omitempty"`
	Raw       string            `json:"raw,omitempty"`
}

// Source labels stamped on events the hub publishes itself instead of
// receiving from a webhook connector. These are not connectors: no
// WebhookConnector CR carries either name and no connector channel is
// provisioned for them — a scheduled tick or a chat message is born directly
// in the target agent's channel. They live here because the hub, the console
// and the MCP server all have to recognise them without importing each other.
const (
	SourceCron = "cron"
	SourceChat = "chat"
)

// IsDirectSource reports whether a connector label is one of the hub-published
// pseudo-sources rather than the name of a real connector.
func IsDirectSource(label string) bool {
	return label == SourceCron || label == SourceChat
}
