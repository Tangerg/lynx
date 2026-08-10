package protocol

import (
	"time"
)

// KnowledgeScope selects which LYRA.md a knowledge operation targets (API.md §4.10).
type KnowledgeScope string

const (
	KnowledgeScopeCWD         KnowledgeScope = "cwd"
	KnowledgeScopeProjectRoot KnowledgeScope = "projectRoot"
	KnowledgeScopeHome        KnowledgeScope = "home"
)

// Valid reports whether s is a known scope.
func (s KnowledgeScope) Valid() bool {
	return s == KnowledgeScopeCWD || s == KnowledgeScopeProjectRoot || s == KnowledgeScopeHome
}

// KnowledgeEntry is one knowledge record (API.md §4.10).
type KnowledgeEntry struct {
	Scope     KnowledgeScope `json:"scope"`
	Content   string         `json:"content"`
	UpdatedAt time.Time      `json:"updatedAt,omitzero"`
}

// GetKnowledgeRequest — knowledge.get body.
type GetKnowledgeRequest struct {
	Scope     KnowledgeScope `json:"scope"`
	Workspace *WorkspaceRef  `json:"workspace,omitempty"`
}

// UpdateKnowledgeRequest — knowledge.update body.
type UpdateKnowledgeRequest struct {
	Scope     KnowledgeScope `json:"scope"`
	Workspace *WorkspaceRef  `json:"workspace,omitempty"`
	Content   string         `json:"content"`
}
