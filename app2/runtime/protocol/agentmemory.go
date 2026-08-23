package protocol

import (
	"time"
)

// AgentMemoryItem is one addressable memory item (API.md §7.x). status is
// active | pending; origin is auto (mined) | user (authored).
type AgentMemoryItem struct {
	ID        string            `json:"id"`
	Scope     AgentMemoryScope  `json:"scope"`
	Content   string            `json:"content"`
	Origin    AgentMemoryOrigin `json:"origin"`
	Status    AgentMemoryStatus `json:"status"`
	Pinned    bool              `json:"pinned"`
	SessionID string            `json:"sessionId,omitempty"`
	Day       string            `json:"day,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// AgentMemoryScope determines which memory set a request or item belongs to.
type AgentMemoryScope string

const (
	AgentMemoryScopeProject AgentMemoryScope = "project"
	AgentMemoryScopeUser    AgentMemoryScope = "user"
)

// AgentMemoryOrigin records whether an item was mined or user-authored.
type AgentMemoryOrigin string

const (
	AgentMemoryOriginAuto AgentMemoryOrigin = "auto"
	AgentMemoryOriginUser AgentMemoryOrigin = "user"
)

// AgentMemoryStatus is the review state visible to a client. Rejected items
// are intentional hidden tombstones and are never projected onto this API.
type AgentMemoryStatus string

const (
	AgentMemoryStatusActive  AgentMemoryStatus = "active"
	AgentMemoryStatusPending AgentMemoryStatus = "pending"
)

// AgentMemoryList is the agentMemory.list result.
type AgentMemoryList struct {
	Items []AgentMemoryItem `json:"items"`
}

// AgentMemoryListRequest — agentMemory.list body. Scope is explicit. Project
// scope requires Workspace; user scope forbids it (contract shape rules).
type AgentMemoryListRequest struct {
	Scope     AgentMemoryScope `json:"scope"`
	Workspace *WorkspaceRef    `json:"workspace,omitempty"`
}

// AgentMemoryReviewRequest — agentMemory.review body. decision is
// "approve" | "reject".
type AgentMemoryReviewRequest struct {
	ID       string                    `json:"id"`
	Decision AgentMemoryReviewDecision `json:"decision"`
}

// AgentMemoryReviewDecision is the explicit user verdict for a pending fact.
type AgentMemoryReviewDecision string

const (
	AgentMemoryReviewApprove AgentMemoryReviewDecision = "approve"
	AgentMemoryReviewReject  AgentMemoryReviewDecision = "reject"
)

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
	Scope     AgentMemoryScope `json:"scope"`
	Workspace *WorkspaceRef    `json:"workspace,omitempty"`
	Content   string           `json:"content"`
}
