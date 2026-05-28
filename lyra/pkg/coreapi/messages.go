package coreapi

import (
	"context"
	"time"
)

// MessageRole mirrors the AG-UI role enum (API.md §6.2).
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleDeveloper MessageRole = "developer"
)

// Message is the wire shape of one message in a session.
type Message struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"sessionId"`
	Role       MessageRole    `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"toolCalls,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToolCall is one entry inside Message.ToolCalls — mirrors AG-UI.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MessagesAPI is the messages.* method group.
type MessagesAPI interface {
	ListMessages(ctx context.Context, in ListMessagesIn) (*Page[Message], error)
	EditMessage(ctx context.Context, in EditMessageIn) (*EditMessageOut, error)
}

// ListMessagesIn — messages.list body. Flat wire shape: sessionId
// alongside the pagination fields (NOT nested under a `query` key).
type ListMessagesIn struct {
	SessionID string `json:"sessionId"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

// PageQuery extracts the pagination fields from ListMessagesIn for
// callsites that want the typed pagination shape.
func (in ListMessagesIn) PageQuery() PageQuery {
	return PageQuery{Limit: in.Limit, Cursor: in.Cursor}
}

// EditMessageIn — messages.edit body. Wire field is `content` (not
// `newContent` — see BACKEND_REVIEW §4.1).
type EditMessageIn struct {
	SessionID string `json:"sessionId"`
	MessageID string `json:"messageId"`
	Content   string `json:"content"`
}

// EditMessageOut — messages.edit result.
type EditMessageOut struct {
	RunID      string `json:"runId"`
	Checkpoint string `json:"checkpoint"`
}
