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

// Session is one conversation, bound to a resolved workspace (API.md §4.1).
type Session struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Status    SessionStatus `json:"status"`
	Model     string        `json:"model"`
	Workspace WorkspaceInfo `json:"workspace"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Favorite  bool          `json:"favorite,omitempty"` // user-pinned; sorts ahead in the session list
	Revision  uint64        `json:"revision"`
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

// CreateSessionRequest — sessions.create body. Workspace is optional and defaults
// to ServerInfo.defaultWorkspace (cold-start zero friction, API.md §7.2).
type CreateSessionRequest struct {
	Workspace *WorkspaceRef `json:"workspace,omitempty"`
	Title     string        `json:"title,omitempty"`
}

// UpdateSessionRequest — sessions.update body. Nil pointers mean "leave alone".
// Setting Workspace is a relocate (gated on features.relocate).
type UpdateSessionRequest struct {
	SessionID        string        `json:"sessionId"`
	ExpectedRevision uint64        `json:"expectedRevision"`
	Title            *string       `json:"title,omitempty"`
	Workspace        *WorkspaceRef `json:"workspace,omitempty"`
	Model            *string       `json:"model,omitempty"`
	Favorite         *bool         `json:"favorite,omitempty"`
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
// transformation. Continuation Runs (resume/edit) open no user message, so it is
// omitted for them.
type DroppedRun struct {
	// Run is a SUMMARY, not a RunRef: this run no longer exists. A RunRef asserts
	// the facts you drive and account for a run by — its active segment, its
	// cumulative metrics, the contract it publishes under — and asserting those
	// about a record the same transaction deleted is reporting accounting for
	// something that is gone.
	Run       RunSummary     `json:"run"`
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
//
// v14 is the Agent Framework Runtime cutover baseline. It admits only artifacts written
// against the single Run/Segment/Interrupt vocabulary and rejects documents
// from the superseded execution lifecycle before any write.
const SessionArtifactVersion = 14

// SessionArtifact is the portable, round-trippable form of a session: its
// identity plus the full conversation — chat messages (the model's context),
// items + runs (the UI transcript), and any structurally bound full tool-result
// bodies. Offloaded item DTOs carry only their bounded preview; ToolResults is
// their single full-body source. Messages remain opaque chat.Message values.
//
// Artifact records intentionally do not reuse the live Session, RunRef, or
// Item response DTOs. A live response includes Runtime-instance-local and derived
// presentation state (for example status and workspace inspection), while an
// artifact is a durable input document. Runs are terminal-only: live and
// waiting executor state is Runtime-instance-local and is therefore not portable.
type SessionArtifact struct {
	Version     int                  `json:"version"`
	Session     ArtifactSession      `json:"session"`
	Messages    []json.RawMessage    `json:"messages"`
	Runs        []ArtifactRun        `json:"runs"`
	Items       []ArtifactItem       `json:"items"`
	ToolResults []ArtifactToolResult `json:"toolResults"`
	// States carries the session-scoped projections a person would notice losing —
	// today the Plan. An archive without them round-trips a conversation and
	// silently drops the work plan attached to it.
	//
	// Only the portable semantic VALUE travels: no revision and no updatedAt. Those
	// are the source runtime's ordering tokens, and carrying them would let an
	// imported value claim a position in the target's revision space that the
	// target never issued — which is how a client comes to ignore a newer value as
	// stale. The importing runtime assigns its own.
	States []ArtifactState `json:"states,omitempty"`
}

// ArtifactStateType is the portable state-key vocabulary. It is the same closed
// first-party set the live stream publishes, because an archive that could carry
// a key the runtime does not own would be restoring a projection with no writer
// and no read.
type ArtifactStateType string

const ArtifactStatePlan ArtifactStateType = "plan"

// ArtifactState is one session-scoped projection's portable value, discriminated
// by its key. At most one entry per type: this is a map of keys to values, and a
// second entry for one key would be two answers to "what was the Plan".
// That rule is an aggregate invariant rather than a schema keyword — the same
// place duplicate item ids are refused.
type ArtifactState struct {
	Type ArtifactStateType `json:"type"`
	Plan []PlanSnapshot    `json:"plan,omitempty"`
}

// ArtifactSession is the durable session identity and user-owned metadata. It
// deliberately excludes live status, revision, and workspace-derived fields.
type ArtifactSession struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Workspace WorkspaceRef `json:"workspace"`
	Model     string       `json:"model"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Favorite  bool         `json:"favorite,omitempty"`
}

// ArtifactRun is the durable terminal record of one run. Outcome is stored as
// the portable terminal fact; the application reconstructs the derived run
// state when restoring it.
type ArtifactRun struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	SpawnedByItemID string `json:"spawnedByItemId,omitempty"`
	// ParentRunID and RootRunID are the child edges, all-or-none with
	// SpawnedByItemID exactly as RunSummary's are — an archive is a durable input
	// document, so a half-linked child would import a tree that cannot be walked.
	ParentRunID string `json:"parentRunId,omitempty"`
	RootRunID   string `json:"rootRunId,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	// Limits and Metrics split the same way the live wire does. The archive has
	// to move with it: leaving the old combined shape here would keep a second,
	// older account of what a run cost alive inside the export format.
	Limits  *ArtifactRunLimits `json:"limits,omitempty"`
	Metrics ArtifactRunMetrics `json:"metrics"`
	// ProtocolProfile is the contract the run published under, required on a root
	// and absent on a child. An import that dropped it would restore a run claiming
	// the Minimal Profile, which is a different run: §14.8 requires the round-trip
	// to preserve it verbatim — never defaulted to empty, never re-derived from the
	// child or interrupt facts, never rewritten to the importing client's
	// capabilities. A child has none of its own; it reads its root's.
	ProtocolProfile *RunProtocolProfile `json:"protocolProfile,omitempty"`
	Outcome         ArtifactOutcome     `json:"outcome"`
	CreatedAt       time.Time           `json:"createdAt"`
	FinishedAt      time.Time           `json:"finishedAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
	MessageMark     int                 `json:"messageMark"`
}

// ArtifactRunLimits is the allowance a portable run was admitted under.
type ArtifactRunLimits struct {
	MaxTotalTokens int64   `json:"maxTotalTokens,omitempty"`
	MaxSteps       int     `json:"maxSteps,omitempty"`
	MaxBudgetUSD   float64 `json:"maxBudgetUsd,omitempty"`
}

// ArtifactRunMetrics is what a portable run consumed.
type ArtifactRunMetrics struct {
	Usage                *ArtifactUsage `json:"usage,omitempty"`
	Steps                int            `json:"steps"`
	ActiveDurationMillis int64          `json:"activeDurationMillis"`
}

// ArtifactOutcome is a non-interrupt terminal fact. Its string discriminator
// is intentionally independent from the live RunOutcome wire union.
type ArtifactOutcome struct {
	Type   ArtifactOutcomeType `json:"type"`
	Error  *ArtifactProblem    `json:"error,omitempty"`
	Detail string              `json:"detail,omitempty"`
}

// ArtifactOutcomeType is the closed terminal vocabulary portable across
// runtime restarts. It deliberately excludes the live-only interrupt outcome.
type ArtifactOutcomeType string

const (
	ArtifactOutcomeCompleted ArtifactOutcomeType = "completed"
	ArtifactOutcomeTimedOut  ArtifactOutcomeType = "timedOut"
	ArtifactOutcomeFailed    ArtifactOutcomeType = "failed"
	ArtifactOutcomeMaxSteps  ArtifactOutcomeType = "maxSteps"
	ArtifactOutcomeMaxBudget ArtifactOutcomeType = "maxBudget"
	ArtifactOutcomeCanceled  ArtifactOutcomeType = "canceled"
	ArtifactOutcomeLost      ArtifactOutcomeType = "lost"
)

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
	ID             string     `json:"id"`
	RunID          string     `json:"runId"`
	Status         ItemStatus `json:"status"`
	CreatedAt      time.Time  `json:"createdAt,omitzero"`
	Type           ItemType   `json:"type"`
	StartedAt      time.Time  `json:"startedAt,omitzero"`
	FinishedAt     time.Time  `json:"finishedAt,omitzero"`
	DurationMillis *int64     `json:"durationMillis,omitempty"`

	Content         []ArtifactContentBlock  `json:"content,omitempty"`
	Text            string                  `json:"text,omitempty"`
	Redacted        bool                    `json:"redacted,omitempty"`
	Question        *ArtifactQuestion       `json:"question,omitempty"`
	Tool            *ArtifactToolInvocation `json:"tool,omitempty"`
	SafetyClass     SafetyClass             `json:"safetyClass,omitempty"`
	Error           *ArtifactProblem        `json:"error,omitempty"`
	Summary         string                  `json:"summary,omitempty"`
	DroppedMessages int                     `json:"droppedMessages,omitempty"`
}

type ArtifactContentBlock struct {
	Type ContentBlockType `json:"type"`
	Text string           `json:"text,omitempty"`
	Mime string           `json:"mime,omitempty"`
	Data string           `json:"data,omitempty"`
}

type ArtifactQuestion struct {
	Fields []ArtifactQuestionField `json:"fields"`
}

type ArtifactQuestionField struct {
	Prompt      string                   `json:"prompt"`
	Header      string                   `json:"header,omitempty"`
	Type        QuestionFieldType        `json:"type"`
	Options     []ArtifactQuestionOption `json:"options,omitempty"`
	Multiple    bool                     `json:"multiple,omitempty"`
	AllowCustom bool                     `json:"allowCustom,omitempty"`
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
	Type              ArtifactProblemType `json:"type"`
	Detail            string              `json:"detail,omitempty"`
	DocURL            string              `json:"docUrl,omitempty"`
	RetryAfterSeconds int                 `json:"retryAfterSeconds,omitempty"`
}

// ArtifactProblemType is the durable transcript error vocabulary. It remains
// separate from ProblemData.Type because artifacts intentionally retain their
// earlier portable names and cannot carry live channel-specific plugin errors.
type ArtifactProblemType string

const (
	ArtifactProblemInternalError       ArtifactProblemType = "internalError"
	ArtifactProblemRunLost             ArtifactProblemType = "runLost"
	ArtifactProblemAgentStuck          ArtifactProblemType = "agentStuck"
	ArtifactProblemRateLimited         ArtifactProblemType = "rateLimited"
	ArtifactProblemInvalidAPIKey       ArtifactProblemType = "invalidApiKey"
	ArtifactProblemTimeout             ArtifactProblemType = "timeout"
	ArtifactProblemProviderUnavailable ArtifactProblemType = "providerUnavailable"
	ArtifactProblemProviderRejected    ArtifactProblemType = "providerRejected"
	ArtifactProblemDeniedByUser        ArtifactProblemType = "deniedByUser"
	ArtifactProblemToolFailed          ArtifactProblemType = "toolFailed"
	ArtifactProblemChildRunCanceled    ArtifactProblemType = "childRunCanceled"
)

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
