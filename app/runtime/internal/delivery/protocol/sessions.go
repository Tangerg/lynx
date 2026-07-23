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
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Status      SessionStatus `json:"status"`
	Model       string        `json:"model"`
	Cwd         string        `json:"cwd"`                   // abs path, server-resolved (symlinks)
	ProjectRoot string        `json:"projectRoot,omitempty"` // derived: nearest .git ancestor, else = cwd
	CwdMissing  bool          `json:"cwdMissing,omitempty"`  // cwd lost on disk → degrade to chat + relocate
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	Favorite    bool          `json:"favorite,omitempty"` // user-pinned; sorts ahead in the session list
	Revision    uint64        `json:"revision"`
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

// GetSessionRequest identifies the session returned by sessions.get.
type GetSessionRequest struct {
	SessionID string `json:"sessionId"`
}

// DeleteSessionRequest identifies the session removed by sessions.delete.
type DeleteSessionRequest struct {
	SessionID string `json:"sessionId"`
}

// CreateSessionRequest — sessions.create body. Cwd is optional; empty
// defaults to ServerInfo.cwd (cold-start zero friction, API.md §7.2).
type CreateSessionRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Title string `json:"title,omitempty"`
}

// UpdateSessionRequest — sessions.update body. Nil pointers mean
// "leave alone". Setting Cwd is a relocate (gated on features.relocate).
type UpdateSessionRequest struct {
	SessionID        string  `json:"sessionId"`
	ExpectedRevision uint64  `json:"expectedRevision"`
	Title            *string `json:"title,omitempty"`
	Cwd              *string `json:"cwd,omitempty"`
	Model            *string `json:"model,omitempty"`
	Favorite         *bool   `json:"favorite,omitempty"`
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
// artifact it doesn't recognize; development builds do not migrate old
// artifacts.
const SessionArtifactVersion = 6

// SessionArtifact is the portable, round-trippable form of a session: its
// identity plus the full conversation — chat messages (the model's context),
// items + runs (the UI transcript), and any structurally bound full tool-result
// bodies. Offloaded item DTOs carry only their bounded preview; ToolResults is
// their single full-body source. Messages remain opaque chat.Message values.
//
// Artifact records intentionally do not reuse the live Session, RunRef, or
// Item response DTOs. A live response includes process-local and derived
// presentation state (for example status and workspace inspection), while an
// artifact is a durable input document. Runs are terminal-only: live and
// interrupted executor state is process-local and is therefore not portable.
type SessionArtifact struct {
	Version     int                  `json:"version"`
	Session     ArtifactSession      `json:"session"`
	Messages    []json.RawMessage    `json:"messages"`
	Runs        []ArtifactRun        `json:"runs"`
	Items       []ArtifactItem       `json:"items"`
	ToolResults []ArtifactToolResult `json:"toolResults"`
}

// ArtifactSession is the durable session identity and user-owned metadata. It
// deliberately excludes live status, revision, and workspace-derived fields.
type ArtifactSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Cwd       string    `json:"cwd"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Favorite  bool      `json:"favorite,omitempty"`
}

// ArtifactRun is the durable terminal record of one run. Outcome is stored as
// the portable terminal fact; the application reconstructs the derived run
// state when restoring it.
type ArtifactRun struct {
	ID              string          `json:"id"`
	SessionID       string          `json:"sessionId"`
	SpawnedByItemID string          `json:"spawnedByItemId,omitempty"`
	Provider        string          `json:"provider,omitempty"`
	Model           string          `json:"model,omitempty"`
	Outcome         ArtifactOutcome `json:"outcome"`
	CreatedAt       time.Time       `json:"createdAt"`
	FinishedAt      time.Time       `json:"finishedAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	MessageMark     int             `json:"messageMark"`
}

// ArtifactOutcome is a non-interrupt terminal fact. Its string discriminator
// is intentionally independent from the live RunOutcome wire union.
type ArtifactOutcome struct {
	Type   string             `json:"type"`
	Result *ArtifactRunResult `json:"result"`
	Detail string             `json:"detail,omitempty"`
}

type ArtifactRunResult struct {
	Usage      *ArtifactUsage   `json:"usage,omitempty"`
	Steps      int              `json:"steps"`
	Error      *ArtifactProblem `json:"error,omitempty"`
	DurationMs int              `json:"durationMs,omitempty"`
}

type ArtifactUsage struct {
	InputTokens      int64                         `json:"inputTokens,omitempty"`
	OutputTokens     int64                         `json:"outputTokens,omitempty"`
	CacheReadTokens  int64                         `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64                         `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  int64                         `json:"reasoningTokens,omitempty"`
	CostUSD          *float64                      `json:"costUsd,omitempty"`
	ByModel          map[string]ArtifactModelUsage `json:"byModel,omitempty"`
}

type ArtifactModelUsage struct {
	InputTokens      int64    `json:"inputTokens,omitempty"`
	OutputTokens     int64    `json:"outputTokens,omitempty"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitempty"`
	CostUSD          *float64 `json:"costUsd,omitempty"`
}

// ArtifactItem is the durable transcript representation. It is not the live
// Item response DTO: archive tool results remain canonical rather than being
// transformed for a particular client presentation.
type ArtifactItem struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Type      string    `json:"type"`

	Content         []ArtifactContentBlock  `json:"content,omitempty"`
	Text            string                  `json:"text,omitempty"`
	Redacted        bool                    `json:"redacted,omitempty"`
	Steps           []ArtifactPlanStep      `json:"steps,omitempty"`
	Question        *ArtifactQuestion       `json:"question,omitempty"`
	Tool            *ArtifactToolInvocation `json:"tool,omitempty"`
	SafetyClass     string                  `json:"safetyClass,omitempty"`
	Error           *ArtifactProblem        `json:"error,omitempty"`
	Summary         string                  `json:"summary,omitempty"`
	DroppedMessages int                     `json:"droppedMessages,omitempty"`
}

type ArtifactContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Mime string `json:"mime,omitempty"`
	Data string `json:"data,omitempty"`
}

type ArtifactPlanStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type ArtifactQuestion struct {
	Prompt string                  `json:"prompt"`
	Fields []ArtifactQuestionField `json:"fields"`
}

type ArtifactQuestionField struct {
	Name     string                   `json:"name"`
	Label    string                   `json:"label"`
	Header   string                   `json:"header,omitempty"`
	Required bool                     `json:"required,omitempty"`
	Type     string                   `json:"type"`
	Options  []ArtifactQuestionOption `json:"options,omitempty"`
	Multiple bool                     `json:"multiple,omitempty"`
}

type ArtifactQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

type ArtifactToolInvocation struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Result    any            `json:"result,omitempty"`
}

type ArtifactProblem struct {
	Type              string `json:"type"`
	Detail            string `json:"detail,omitempty"`
	DocURL            string `json:"docUrl,omitempty"`
	Retryable         bool   `json:"retryable,omitempty"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}

// ArtifactToolResult carries the single full-body source for an offloaded tool
// item. ItemID binds it structurally; Preview is the model-history replacement
// restored into the transcript while Body remains available to presentation and
// read_tool_result.
type ArtifactToolResult struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"itemId"`
	ToolName  string    `json:"toolName"`
	Preview   string    `json:"preview"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
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
