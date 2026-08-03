package ainselapishared

import "time"

type IssueData struct {
	Label string       `json:"label,omitempty"` // the specific label just added
	Issue IssuePayload `json:"issue"`
}

type IssueCommentData struct {
	Comment CommentPayload `json:"comment"`
	Issue   IssuePayload   `json:"issue"`
}

type PullRequestData struct {
	Label       string             `json:"label,omitempty"` // the specific label just added
	PullRequest PullRequestPayload `json:"pull_request"`
}

type PullRequestCommentData struct {
	Comment     CommentPayload     `json:"comment"`
	PullRequest PullRequestPayload `json:"pull_request"`
}

type PullRequestReviewData struct {
	Review      ReviewPayload      `json:"review"`
	PullRequest PullRequestPayload `json:"pull_request"`
}

type PushData struct {
	Ref     string          `json:"ref"`
	Before  string          `json:"before"`
	After   string          `json:"after"`
	Commits []CommitPayload `json:"commits"`
}

type IssuePayload struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
}

type CommentPayload struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type PullRequestPayload struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	State  string   `json:"state"`
	Merged bool     `json:"merged"`
	Head   string   `json:"head"`
	Base   string   `json:"base"`
	Labels []string `json:"labels"`
}

type ReviewPayload struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
	Type string `json:"type"`
}

type CommitPayload struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Author  string `json:"author"`
}

type WorkflowRunData struct {
	RunTitle         string `json:"run_title"`
	WorkflowID       string `json:"workflow_id"`
	HTMLURL          string `json:"html_url"`
	TriggerUserLogin string `json:"trigger_user_login"`
	TriggerEvent     string `json:"trigger_event"`
	Status           string `json:"status"`
}

// ChatMessageData is the payload for chat.message events. These events
// are published by the hub when a human user sends a message to an agent
// via the chat API. The agent responds via the mcp__chat__send_reply MCP
// tool, not by posting a Forgejo comment.
type ChatMessageData struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}
