package protocol

import (
	"context"
	"time"
)

// SessionStatus mirrors the wire enum (API.md §4.1).
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusWaiting SessionStatus = "waiting"
	SessionStatusIdle    SessionStatus = "idle"
)

// Session is one conversation, bound to a working directory (API.md §4.1).
type Session struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Status      SessionStatus  `json:"status"`
	Model       string         `json:"model"`
	Cwd         string         `json:"cwd"`                   // abs path, server-resolved (symlinks)
	ProjectRoot string         `json:"projectRoot,omitempty"` // derived: nearest .git ancestor, else = cwd
	CwdMissing  bool           `json:"cwdMissing,omitempty"`  // cwd lost on disk → degrade to chat + relocate
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Usage       *Usage         `json:"usage,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

// Project is the distinct-Session.cwd derived view (API.md §4.1). No
// opaque id, no active flag — identity is the cwd itself.
type Project struct {
	Cwd          string     `json:"cwd"`
	Name         string     `json:"name"`
	ProjectRoot  string     `json:"projectRoot,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	SessionCount int        `json:"sessionCount"`
	LastActiveAt *time.Time `json:"lastActiveAt,omitzero"`
	CwdMissing   bool       `json:"cwdMissing,omitempty"`
}

// Sessions is the sessions.* method group (API.md §7.2).
type Sessions interface {
	ListSessions(ctx context.Context, q PageQuery) (*Page[Session], error)
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	CreateSession(ctx context.Context, in CreateSessionRequest) (*Session, error)
	UpdateSession(ctx context.Context, in UpdateSessionRequest) (*Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ForkSession(ctx context.Context, in ForkSessionRequest) (*Session, error)
	// RollbackSession discards the runs after a kept boundary, truncating the
	// session's history at a run granularity (AUX_API §4.1). Destructive +
	// in-place — it mutates the session rather than producing a copy (that's
	// fork). Rejected with session_busy while a run is in flight.
	RollbackSession(ctx context.Context, in RollbackSessionRequest) (*RollbackSessionResponse, error)
	ExportSession(ctx context.Context, in ExportSessionRequest) (*ExportSessionResponse, error)
}

// CreateSessionRequest — sessions.create body. Cwd is optional; empty
// defaults to ServerInfo.cwd (cold-start zero friction, API.md §7.2).
type CreateSessionRequest struct {
	Cwd      string         `json:"cwd,omitempty"`
	Title    string         `json:"title,omitempty"`
	Model    string         `json:"model,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateSessionRequest — sessions.update body. Nil pointers mean
// "leave alone". Setting Cwd is a relocate (gated on features.relocate).
type UpdateSessionRequest struct {
	SessionID string          `json:"sessionId"`
	Title     *string         `json:"title,omitempty"`
	Cwd       *string         `json:"cwd,omitempty"`
	Model     *string         `json:"model,omitempty"`
	Metadata  *map[string]any `json:"metadata,omitempty"`
}

// ForkSessionRequest — sessions.fork body (AUX_API §4.2). Omit fromRunId for a
// whole-conversation fork; give it to truncate-copy up to and including that
// run boundary. Inherits the source cwd.
type ForkSessionRequest struct {
	SessionID string `json:"sessionId"`
	FromRunID string `json:"fromRunId,omitempty"`
	Title     string `json:"title,omitempty"`
}

// RollbackSessionRequest — sessions.rollback body (AUX_API §4.1). ToRunID is
// inclusive-keep: the last ROOT run to keep, everything after it is dropped
// (its continuation chain + subagent subtree + dangling interrupts go too).
// Omit ToRunID to drop every run and return to an empty session ("edit the
// first message"). A non-root / continuation ToRunID is invalid_params.
type RollbackSessionRequest struct {
	SessionID string `json:"sessionId"`
	ToRunID   string `json:"toRunId,omitempty"`
}

// RollbackSessionResponse — sessions.rollback result. DroppedRuns lists what
// was removed (newest-relevant first is not required; the server returns drop
// order) so the client can reconcile its view and re-populate the composer.
type RollbackSessionResponse struct {
	Session     *Session     `json:"session"`
	DroppedRuns []DroppedRun `json:"droppedRuns"`
}

// DroppedRun is one run sessions.rollback removed (AUX_API §4.1). UserInput is
// the dropped run's opening userMessage content — same shape as
// StartRunRequest.input, so the client can re-populate the composer with zero
// transformation. Continuation runs (resume/edit) open no user turn, so it's
// omitted for them.
type DroppedRun struct {
	Run       RunRef         `json:"run"`
	UserInput []ContentBlock `json:"userInput,omitempty"`
}

// ExportFormat enumerates sessions.export output formats.
type ExportFormat string

const (
	ExportFormatMarkdown ExportFormat = "md"
	ExportFormatJSON     ExportFormat = "json"
)

// ExportSessionRequest — sessions.export body.
type ExportSessionRequest struct {
	SessionID string       `json:"sessionId"`
	Format    ExportFormat `json:"format,omitempty"`
}

// ExportSessionResponse — sessions.export result. URL points at a
// transport file channel; bytes are fetched out of band (API.md §7.2).
type ExportSessionResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}
