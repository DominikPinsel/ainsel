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
