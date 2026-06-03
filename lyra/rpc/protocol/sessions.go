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
	LastActiveAt *time.Time `json:"lastActiveAt,omitempty"`
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

// ForkSessionRequest — sessions.fork body. Forks at an item boundary,
// inheriting the source cwd.
type ForkSessionRequest struct {
	SessionID  string `json:"sessionId"`
	FromItemID string `json:"fromItemId,omitempty"`
	Title      string `json:"title,omitempty"`
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
