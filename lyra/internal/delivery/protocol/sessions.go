package protocol

import (
	"context"
	"encoding/json"
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
	// ExportSession serializes a session to a portable artifact (AUX_API §4.3):
	// format=json yields a round-trippable SessionArtifact (consumed by
	// ImportSession), format=md a human-readable transcript. Gated by
	// features.sessionExport.
	ExportSession(ctx context.Context, in ExportSessionRequest) (*ExportSessionResponse, error)
	// ImportSession recreates a session from a SessionArtifact under its
	// ORIGINAL id (restore semantics — overwrites one already present), so an
	// exported session round-trips faithfully. Gated by features.sessionExport.
	ImportSession(ctx context.Context, in ImportSessionRequest) (*ImportSessionResponse, error)
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
	// RestoreType selects what the rollback rewinds (AUX_API §4.3), default
	// "history". "files"/"both" restore the working tree to ToRunID's
	// checkpoint and require ToRunID + features.checkpoints; "both" is atomic
	// (files first — if they fail, history is left untouched).
	RestoreType RestoreType `json:"restoreType,omitempty"`
}

// RestoreType selects what sessions.rollback rewinds.
type RestoreType string

const (
	RestoreHistory RestoreType = "history" // chat history only (default; files untouched)
	RestoreFiles   RestoreType = "files"   // working-tree files only (history untouched)
	RestoreBoth    RestoreType = "both"    // both, atomically (files first)
)

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

// ExportSessionRequest — sessions.export body. Format defaults to json.
type ExportSessionRequest struct {
	SessionID string       `json:"sessionId"`
	Format    ExportFormat `json:"format,omitempty"`
}

// ExportSessionResponse — sessions.export result, returned INLINE (lyra is a
// local loopback runtime, so there is no out-of-band file channel / giant-
// payload concern). For format=json, Artifact is the round-trippable bundle
// sessions.import consumes; for format=md, Markdown is a human-readable
// transcript (not re-importable). Exactly one is populated, per Format.
type ExportSessionResponse struct {
	Format   ExportFormat     `json:"format"`
	Artifact *SessionArtifact `json:"artifact,omitempty"`
	Markdown string           `json:"markdown,omitempty"`
}

// SessionArtifactVersion is the artifact schema version. Import rejects an
// artifact it doesn't recognize, so a future breaking change bumps this.
const SessionArtifactVersion = 1

// SessionArtifact is the portable, round-trippable form of a session: its
// metadata plus the full conversation — chat messages (the model's context),
// and the items + runs (the UI transcript). Messages are opaque chat.Message
// blobs; items/runs carry the storage keys explicitly so import reconstructs
// them without peeking inside the wire blob.
type SessionArtifact struct {
	Version  int               `json:"version"`
	Session  Session           `json:"session"`
	Messages []json.RawMessage `json:"messages"`
	Runs     []ArtifactRun     `json:"runs"`
	Items    []ArtifactItem    `json:"items"`
}

// ArtifactRun is one run record in a SessionArtifact — the wire RunRef blob
// plus the storage-side fields (watermark, updatedAt) not carried in RunRef.
type ArtifactRun struct {
	RunID     string          `json:"runId"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Mark      int             `json:"mark"` // chat-memory watermark for rollback/fork boundaries (-1 = unknown)
	Run       json.RawMessage `json:"run"`  // protocol.RunRef blob
}

// ArtifactItem is one item record in a SessionArtifact — the wire Item blob
// plus the storage keys (run id, item id, createdAt) needed to re-persist it.
type ArtifactItem struct {
	RunID     string          `json:"runId"`
	ItemID    string          `json:"itemId"`
	CreatedAt time.Time       `json:"createdAt"`
	Item      json.RawMessage `json:"item"` // protocol.Item blob
}

// ImportSessionRequest — sessions.import body. Restore semantics: the session
// is recreated under Artifact.Session.ID (overwriting one already present).
type ImportSessionRequest struct {
	Artifact SessionArtifact `json:"artifact"`
}

// ImportSessionResponse — sessions.import result: the restored session.
type ImportSessionResponse struct {
	Session *Session `json:"session"`
}
