// Package chat provides storage and retrieval of chat sessions between
// human users and AInsel agents. A session is a conversation thread with
// a specific agent; messages within a session have a role (user, assistant,
// or status) and content.
package chat

import "time"

// MessageRole constants for chat messages.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleStatus    = "status"
)

// Session is a chat conversation between a human user and an agent.
type Session struct {
	// ID is the unique session identifier. Format: "sess-<8 hex>".
	ID string `json:"id"`

	// Name is the human-readable display name of the session.
	// Defaults to the session ID at creation time; editable via the API.
	Name string `json:"name"`

	// AgentName is the name of the agent this session is with.
	AgentName string `json:"agentName"`

	// UserID is the identity of the human user (OIDC sub).
	UserID string `json:"userId"`

	// CreatedAt is when the session was started.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp of the last message in the session.
	UpdatedAt time.Time `json:"updatedAt"`

	// Messages is the message history, populated only when fetching a
	// single session by ID. Empty in list responses.
	Messages []Message `json:"messages,omitempty"`
}

// Message is a single chat message within a session.
type Message struct {
	// ID is the database row ID.
	ID int64 `json:"id"`

	// SessionID is the session this message belongs to.
	SessionID string `json:"sessionId"`

	// Role is one of: user, assistant, status.
	Role string `json:"role"`

	// Content is the message text (markdown supported for assistant role).
	Content string `json:"content"`

	// Tokens is the LLM token count for this message (0 for user/status).
	Tokens int `json:"tokens"`

	// CreatedAt is when the message was stored.
	CreatedAt time.Time `json:"createdAt"`
}

// CreateSessionRequest is the input for creating a new chat session.
type CreateSessionRequest struct {
	// AgentName is the name of the agent to chat with. Required.
	AgentName string `json:"agentName"`
}

// UpdateSessionRequest is the input for updating a chat session's metadata.
type UpdateSessionRequest struct {
	// Name is the new display name for the session. Required, non-empty.
	Name string `json:"name"`
}

// CreateMessageRequest is the input for appending a message to a session.
type CreateMessageRequest struct {
	// Role is the message role: user, assistant, or status. Required.
	Role string `json:"role"`

	// Content is the message text. Required.
	Content string `json:"content"`

	// Tokens is the LLM token count (optional, defaults to 0).
	Tokens int `json:"tokens,omitempty"`
}