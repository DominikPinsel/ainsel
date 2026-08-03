// Package triggers owns the trigger and cron trigger database layer.
// It replaces the former Kubernetes CRD-based storage with Postgres tables,
// consistent with how personas, skills, and MCP servers are stored.
package triggers

import (
	"time"

	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// Trigger is the canonical trigger record from the database.
type Trigger struct {
	ID             string             `json:"id"`
	DisplayName    string             `json:"name"`
	AgentRef       string             `json:"agentRef"`
	ConnectorRef   string             `json:"connectorRef"`
	Filters        []ainselapishared.Filter `json:"filters,omitempty"`
	AgentValid     bool               `json:"agentValid"`
	ConnectorValid bool               `json:"connectorValid"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// CronTrigger is the canonical cron trigger record from the database.
type CronTrigger struct {
	ID            string     `json:"id"`
	DisplayName   string     `json:"name"`
	AgentRef      string     `json:"agentRef"`
	Schedule      string     `json:"schedule"`
	Prompt        string     `json:"prompt"`
	Enabled       bool       `json:"enabled"`
	AgentValid    bool       `json:"agentValid"`
	ScheduleValid bool       `json:"scheduleValid"`
	LastRun       *time.Time `json:"lastRun,omitempty"`
	NextRun       *time.Time `json:"nextRun,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}
