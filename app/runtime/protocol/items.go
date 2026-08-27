package protocol

import (
	"time"
)

// ItemScopeType discriminates [ItemListScope]: which collection of items is being
// paged.
type ItemScopeType string

const (
	// ItemScopeSession — the whole session timeline, every run in it.
	ItemScopeSession ItemScopeType = "session"
	// ItemScopeRun — one run's own items, optionally with its subtree's.
	ItemScopeRun ItemScopeType = "run"
)

// ItemListScope is the closed union naming what items.list pages. It is required:
// a read with no scope is not a read of "everything", it is a request that never
// said what it wanted.
//
// The variant decides which fields are read — session scope reads sessionId, run
// scope reads runId and includeDescendants. A run id already locates its session,
// so run scope carries no sessionId: a second way to say where the run lives is a
// second thing that can be wrong.
type ItemListScope struct {
	Type ItemScopeType `json:"type"`
	// SessionID is the session scope's subject.
	SessionID string `json:"sessionId,omitempty"`
	// RunID is the run scope's subject, root or child.
	RunID string `json:"runId,omitempty"`
	// IncludeDescendants adds the items of the run's subtree at any depth. Legal
	// only in run scope — the session timeline already holds every descendant, so
	// asking there would name a narrowing that does not narrow.
	IncludeDescendants bool `json:"includeDescendants,omitempty"`
}

// ItemOrder is the direction items.list walks the durable sequence.
type ItemOrder string

const (
	// ItemOrderAsc — earliest first. The default, and what a reducer replaying a
	// session into state needs: the order the runtime produced.
	ItemOrderAsc ItemOrder = "asc"
	// ItemOrderDesc — newest first. What a long session's first screen needs: the
	// tail, without paging the whole history to reach it.
	ItemOrderDesc ItemOrder = "desc"
)

// ListItemsRequest — items.list body.
type ListItemsRequest struct {
	Scope ItemListScope `json:"scope"`
	// Order defaults to asc. It is part of the cursor's identity: an anchor from a
	// page read forwards cannot continue one read backwards.
	Order ItemOrder `json:"order,omitempty"`
	PageQuery
}

// ListItemsResponse — items.list result: a Page[Item] (`data` +
// `nextCursor`) embedded so every list method reads `resp.data`, plus the run
// summaries needed to rebuild the run tree (§7.4 / §10.3 —
// `Page<Item> & { runs }`). The embedded Page inlines `data`/`nextCursor`
// onto the wire.
//
// Summaries, not full RunRefs: threading items onto their runs needs identity
// and lifecycle, and a page can carry many runs. Metering and protocol facts
// grow with the model and the subtree, so they stay on the per-run read that
// asks for them.
//
// Runs holds the Runs THIS page's items reference plus their ancestor chains. It
// is not the session's run list: a client merging summaries across pages by runId
// rebuilds the connected tree it has actually seen, and a long session does not
// pay for unrelated Runs. Accounting over a whole session is runs.list.
type ListItemsResponse struct {
	Page[Item]
	Runs []RunSummary `json:"runs"`
}

// ItemStatus is the lifecycle status of an Item (API.md §4.3).
type ItemStatus string

const (
	ItemStatusRunning    ItemStatus = "running" // in-progress (§2.3: "running" everywhere)
	ItemStatusCompleted  ItemStatus = "completed"
	ItemStatusIncomplete ItemStatus = "incomplete" // interrupted/canceled before completion
)

// ItemType discriminates the Item union (API.md §4.3).
type ItemType string

const (
	ItemTypeUserMessage  ItemType = "userMessage"
	ItemTypeAgentMessage ItemType = "agentMessage"
	ItemTypeReasoning    ItemType = "reasoning"
	ItemTypeQuestion     ItemType = "question"
	ItemTypeToolCall     ItemType = "toolCall"
	ItemTypeCompaction   ItemType = "compaction"
)

// MessagePhase is the authored role of an AgentMessage inside one Run. A
// commentary message precedes more model or Tool work; finalAnswer is the
// terminal response. It is absent from the provisional running shell and
// required on a terminal AgentMessage.
type MessagePhase string

const (
	MessagePhaseCommentary  MessagePhase = "commentary"
	MessagePhaseFinalAnswer MessagePhase = "finalAnswer"
)

// SafetyClass is a tool's mutation risk (API.md §4.4): safe (read-only),
// write (mutates the workspace), exec (runs arbitrary commands), network
// (reaches off-host). Carried on a toolCall Item and on a client-supplied
// ToolSpec.
type SafetyClass string

const (
	SafetyClassSafe    SafetyClass = "safe"
	SafetyClassWrite   SafetyClass = "write"
	SafetyClassExec    SafetyClass = "exec"
	SafetyClassNetwork SafetyClass = "network"
)

// ContentBlockType discriminates a ContentBlock (API.md §4.3).
type ContentBlockType string

const (
	ContentBlockText  ContentBlockType = "text"
	ContentBlockImage ContentBlockType = "image"
)

// QuestionFieldType is the input shape of a QuestionField (API.md §4.3).
type QuestionFieldType string

const (
	QuestionFieldText   QuestionFieldType = "text"
	QuestionFieldChoice QuestionFieldType = "choice"
)

// DiffRowType discriminates a structured diff row (API.md §4.5).
type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

// Item is one wire projection in a Run stream or transcript read (API.md §4.3).
// A tag-discriminated union: Type selects which optional fields apply. Durable
// user/message/reasoning/question/compaction facts are complete; a provisional
// AgentMessage/Reasoning start exists only as a stream rendering anchor, while
// ToolCall is the only variant with a durable running lifecycle.
//
//	userMessage                → Content
//	agentMessage               → Phase, Content
//	reasoning                  → Text, Redacted
//	question                   → Question
//	toolCall                   → Tool, SafetyClass, ApprovalDecision, Error
//	compaction                 → Summary, DroppedMessages
type Item struct {
	ID     string     `json:"id"`
	RunID  string     `json:"runId"`
	Status ItemStatus `json:"status"`
	// CreatedAt belongs to non-ToolCall variants. A ToolCall starts at StartedAt;
	// the union contract forbids carrying both names for the same instant.
	CreatedAt time.Time `json:"createdAt,omitzero"`
	Type      ItemType  `json:"type"`
	// ToolCall lifecycle timing. StartedAt is when the request became a visible
	// Item and FinishedAt is when that Item settled. DurationMillis is present
	// only when the Runtime knows the exact Tool execution interval; it excludes
	// approval and other pre-execution waits and may therefore be shorter than
	// FinishedAt-StartedAt.
	StartedAt      time.Time `json:"startedAt,omitzero"`
	FinishedAt     time.Time `json:"finishedAt,omitzero"`
	DurationMillis *int64    `json:"durationMillis,omitempty"`

	Content     []ContentBlock  `json:"content,omitempty"`
	Phase       MessagePhase    `json:"phase,omitempty"`
	Text        string          `json:"text,omitempty"`
	Redacted    bool            `json:"redacted,omitempty"`
	Question    *Question       `json:"question,omitempty"`
	Tool        *ToolInvocation `json:"tool,omitempty"`
	SafetyClass SafetyClass     `json:"safetyClass,omitempty"`
	// ApprovalDecision is present only when this exact ToolCall crossed a human
	// approval boundary. Auto-approved calls carry no decision.
	ApprovalDecision ApprovalDecision `json:"approvalDecision,omitempty"`
	Error            *ProblemData     `json:"error,omitempty"` // tool-level failure (API.md §4.3)
	// Summary / DroppedMessages describe a compaction Item at a safe model-call
	// or Run boundary. DroppedMessages is the net history reduction
	// (messages before − after); Summary is an optional human note (currently
	// left empty — the summary text is folded into the rewritten history).
	Summary         string `json:"summary,omitempty"`         // compaction
	DroppedMessages int    `json:"droppedMessages,omitempty"` // compaction
}

// ContentBlock is one block of message content (API.md §4.3).
//
//	text  → Text
//	image → Mime + Data (inline base64)
//
// Images are carried inline: Data is the raw base64 of the image bytes
// (no data: URL prefix) and Mime is its media type ("image/png", …). The
// pair maps directly onto a core media.Media — Mime parses to the MIME and
// Data is the base64 payload — so no attachment indirection is needed.
type ContentBlock struct {
	Type ContentBlockType `json:"type"` // see ContentBlockType
	Text string           `json:"text,omitempty"`
	Mime string           `json:"mime,omitempty"`
	Data string           `json:"data,omitempty"`
}

// Question is one ordered set of required clarifying fields (API.md §4.3).
// InterruptResponseValue.answers uses this same order; no derivable field IDs
// or dynamic map keys travel on the wire.
type Question struct {
	Fields  []QuestionField `json:"fields"`
	Answers [][]string      `json:"answers,omitempty"` // accepted response, in Fields order
}

// QuestionField is one field of a Question. Type selects the shape:
//
//	text   → (no extra)
//	choice → Options, Multiple, AllowCustom
type QuestionField struct {
	Prompt      string            `json:"prompt"`
	Header      string            `json:"header,omitempty"` // ≤12-char chip
	Type        QuestionFieldType `json:"type"`             // see QuestionFieldType
	Options     []QuestionOption  `json:"options,omitempty"`
	Multiple    bool              `json:"multiple,omitempty"`
	AllowCustom bool              `json:"allowCustom,omitempty"`
}

// QuestionOption is one choice option (API.md §4.3).
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

// ToolInvocation is the domain-neutral tool envelope (API.md §4.4). The
// core knows exactly ONE tool shape — not a union: Name is identity,
// Arguments is the parsed JSON object, Result is best-effort JSON output.
// Result is normalized before transcript persistence and delivery forwards its
// canonical JSON value unchanged. Adding a tool requires no protocol union
// change.
//
// Hard constraints (§4.4.1):
//   - Arguments is ALWAYS a JSON object, never a JSON string (no double
//     escaping). Streaming partial args arrive via ItemDelta.argumentsTextDelta
//     and are unmarshaled into Arguments at item.completed / the approval
//     payload (§4.8).
//   - Result is best-effort JSON, NEVER double-encoded; absent on
//     item.started, authoritative and persisted on item.completed (§5.2). The
//     command-output preview rides ItemDelta.toolOutput, whose terminal value
//     is result.output (§5.2) — clients must not treat the streamed
//     accumulation as the source of truth.
//   - Tool-level failure does NOT go in Result — it rides the toolCall
//     Item's Error + status:"incomplete" (§4.3 / §8).
type ToolInvocation struct {
	Name      string         `json:"name"`             // tool identity (stable); an MCP tool's is its LOSSY model-facing name — see API.md §4.4, authored by mcpserver.ToolName
	Arguments map[string]any `json:"arguments"`        // parsed JSON object (always present; never a JSON string)
	Result    any            `json:"result,omitempty"` // best-effort JSON; absent on item.started, authoritative on item.completed
}

// DiffRow is one structured row of a unified diff (API.md §4.5). Code
// is plain text — the client highlights.
//
//	hunk    → Text
//	context → LeftLine, RightLine, Code
//	added   → RightLine, Code
//	deleted → LeftLine, Code
type DiffRow struct {
	Type      DiffRowType `json:"type"` // see DiffRowType
	Text      string      `json:"text,omitempty"`
	LeftLine  int         `json:"leftLine,omitempty"`
	RightLine int         `json:"rightLine,omitempty"`
	Code      string      `json:"code,omitempty"`
}

// ModelUsage is one model's usage slice (API.md §4.6): provider-reported
// inclusive totals (inputTokens incl. cacheRead, outputTokens incl.
// reasoning) plus the non-overlapping sub-items, each tracked independently
// so the client never subtracts. costUsd is the total at the top level and
// per-model in byModel; omitted (not faked to 0) when the model isn't priced.
type ModelUsage struct {
	InputTokens      int64    `json:"inputTokens,omitempty"`
	OutputTokens     int64    `json:"outputTokens,omitempty"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitempty"`
	CostUSD          *float64 `json:"costUsd,omitempty"`
}

// Usage is cumulative token usage (API.md §4.6): the embedded ModelUsage is
// the total (incl. the top-level costUsd = total cost), plus an optional
// per-model breakdown. byModel entries are the same shape (cache fields
// included — symmetric with the total).
type Usage struct {
	ModelUsage
	ByModel map[string]ModelUsage `json:"byModel,omitempty"`
}
