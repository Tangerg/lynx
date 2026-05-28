package coreapi

import (
	"context"
	"time"
)

// SessionStatus mirrors the wire enum (API.md §6.2).
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusWaiting SessionStatus = "waiting"
	SessionStatusIdle    SessionStatus = "idle"
)

// Session is the wire shape of one conversation. Metadata is
// `Record<string, unknown>` on the wire (any JSON value), the internal
// store may be narrower — lyracore bridges the type at the boundary.
type Session struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Status        SessionStatus  `json:"status"`
	Model         string         `json:"model"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	LastMessageAt *time.Time     `json:"lastMessageAt,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	Pinned        bool           `json:"pinned,omitempty"`
	Archived      bool           `json:"archived,omitempty"`
}

// Sessions is the sessions.* method group.
type Sessions interface {
	ListSessions(ctx context.Context, q PageQuery) (*Page[Session], error)
	GetSession(ctx context.Context, id string) (*Session, error)
	CreateSession(ctx context.Context, in CreateSessionRequest) (*Session, error)
	UpdateSession(ctx context.Context, in UpdateSessionRequest) (*Session, error)
	DeleteSession(ctx context.Context, id string) error
	ForkSession(ctx context.Context, in ForkSessionRequest) (*Session, error)
	ExportSession(ctx context.Context, in ExportSessionRequest) (*ExportSessionResponse, error)
}

// CreateSessionRequest — sessions.create body.
type CreateSessionRequest struct {
	Title    string         `json:"title,omitempty"`
	Model    string         `json:"model,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateSessionRequest — sessions.update body. Carries the target ID
// plus optional patch fields (flat wire shape, no nested envelope).
// Nil pointers mean "leave alone"; non-nil applies the value. Metadata
// is full replacement.
type UpdateSessionRequest struct {
	ID       string             `json:"id"`
	Title    *string            `json:"title,omitempty"`
	Pinned   *bool              `json:"pinned,omitempty"`
	Archived *bool              `json:"archived,omitempty"`
	Metadata *map[string]any    `json:"metadata,omitempty"`
}

// ForkSessionRequest — sessions.fork body. ParentID is the source session
// being forked from (not the new id). See BACKEND_REVIEW §5.1 for the
// naming rationale.
type ForkSessionRequest struct {
	ParentID    string `json:"parentId"`
	AtMessageID string `json:"atMessageId"`
}

// ExportFormat enumerates sessions.export output formats.
type ExportFormat string

const (
	ExportFormatMarkdown ExportFormat = "md"
	ExportFormatJSON     ExportFormat = "json"
)

// ExportSessionRequest — sessions.export body.
type ExportSessionRequest struct {
	ID     string       `json:"id"`
	Format ExportFormat `json:"format"`
}

// ExportSessionResponse — sessions.export result. URL points at a
// transport-specific download endpoint; the caller fetches the bytes
// through that URL out of band (API.md §5.2).
type ExportSessionResponse struct {
	URL string `json:"url"`
}
