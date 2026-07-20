package protocol

import (
	"context"
	"time"
)

// AgentMemory is the agentMemory.* method group — the human-in-the-loop review
// surface over the agent's self-maintained memory. The agent proposes durable
// facts it mined from a session; they wait as pending until the user approves
// them, and only approved memory is injected into future prompts or returned by
// the memory_search tool. This is distinct from the Memory group, which edits
// the user-authored LYRA.md cascade.
type AgentMemory interface {
	// ListAgentMemory returns the project's active + pending memory items,
	// pending first. Rejected tombstones are hidden.
	ListAgentMemory(ctx context.Context, in AgentMemoryListRequest) (*AgentMemoryList, error)
	// ReviewAgentMemory approves (→ active) or rejects (→ hidden tombstone) a
	// pending proposal.
	ReviewAgentMemory(ctx context.Context, in AgentMemoryReviewRequest) error
	// UpdateAgentMemory edits an item's content and/or pins it. Both fields are
	// optional; a nil field is left unchanged.
	UpdateAgentMemory(ctx context.Context, in AgentMemoryUpdateRequest) (*AgentMemoryItem, error)
	// DeleteAgentMemory removes an item outright.
	DeleteAgentMemory(ctx context.Context, in AgentMemoryItemRequest) error
	// AddAgentMemory stores a user-authored active memory item.
	AddAgentMemory(ctx context.Context, in AgentMemoryAddRequest) (*AgentMemoryItem, error)
}

// AgentMemoryItem is one addressable memory item (API.md §7.x). status is
// active | pending; origin is auto (mined) | user (authored).
type AgentMemoryItem struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	Content   string    `json:"content"`
	Origin    string    `json:"origin"`
	Status    string    `json:"status"`
	Pinned    bool      `json:"pinned"`
	SessionID string    `json:"sessionId,omitempty"`
	Day       string    `json:"day,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentMemoryList is the agentMemory.list result.
type AgentMemoryList struct {
	Items []AgentMemoryItem `json:"items"`
}

// AgentMemoryListRequest — agentMemory.list body. scope is "project" (default)
// or "user"; cwd resolves the project for the project scope.
type AgentMemoryListRequest struct {
	Scope string `json:"scope,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
}

// AgentMemoryReviewRequest — agentMemory.review body. decision is
// "approve" | "reject".
type AgentMemoryReviewRequest struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
}

// AgentMemoryUpdateRequest — agentMemory.update body. A nil field is unchanged.
type AgentMemoryUpdateRequest struct {
	ID      string  `json:"id"`
	Content *string `json:"content,omitempty"`
	Pinned  *bool   `json:"pinned,omitempty"`
}

// AgentMemoryItemRequest — agentMemory.delete body.
type AgentMemoryItemRequest struct {
	ID string `json:"id"`
}

// AgentMemoryAddRequest — agentMemory.add body.
type AgentMemoryAddRequest struct {
	Scope   string `json:"scope,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Content string `json:"content"`
}
