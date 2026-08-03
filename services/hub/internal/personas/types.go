// Package personas owns the persona registry: persistence (Store),
// ConfigMap rendering (Reconciler), and the Service facade that wires
// the two together for handlers.
package personas

import "time"

// Persona is the canonical persona record from the database, with the
// current version's text inlined for convenience.
type Persona struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CurrentVersion int       `json:"currentVersion"`
	Text           string    `json:"text"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// PersonaSummary is the listing projection: same as Persona without
// the (potentially large) text body.
type PersonaSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CurrentVersion int       `json:"currentVersion"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Version is a single persona_versions row.
type Version struct {
	PersonaID     string    `json:"personaId"`
	VersionNumber int       `json:"versionNumber"`
	Text          string    `json:"text"`
	CreatedAt     time.Time `json:"createdAt"`
}

// VersionSummary omits the text body for listing.
type VersionSummary struct {
	PersonaID     string    `json:"personaId"`
	VersionNumber int       `json:"versionNumber"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Referrer identifies an Agent CR that references a persona by ID.
// Returned by Service.Delete when the delete is refused.
type Referrer struct {
	AgentName string `json:"agentName"`
}
