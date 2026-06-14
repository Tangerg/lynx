package protocol

import (
	"context"
	"encoding/json"
	"time"
)

// Items is the items.* method group — durable conversation history
// (API.md §7.4). History = the completed Item sequence; there is no
// separate Message type.
type Items interface {
	// ListItems returns completed items plus the RunRef records needed
	// to reconstruct their run tree + continuation chain (API.md §7.4).
	ListItems(ctx context.Context, in ListItemsRequest) (*ListItemsResponse, error)
}

// ListItemsRequest — items.list body.
type ListItemsRequest struct {
	SessionID string `json:"sessionId"`
	PageQuery
}

// ListItemsResponse — items.list result: a Page[Item] (`data` +
// `nextCursor`) embedded so every list method reads `resp.data`, plus the
// RunRefs needed to rebuild the run tree (API.md §7.4 / §10.3 —
// `Page<Item> & { runs }`). The embedded Page inlines `data`/`nextCursor`
// onto the wire.
type ListItemsResponse struct {
	Page[Item]
	Runs []RunRef `json:"runs"`
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
	ItemTypePlan         ItemType = "plan"
	ItemTypeQuestion     ItemType = "question"
	ItemTypeToolCall     ItemType = "toolCall"
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

// Valid reports whether t is a known content-block type.
func (t ContentBlockType) Valid() bool { return t == ContentBlockText || t == ContentBlockImage }

// PlanStepStatus is the lifecycle of one PlanStep (API.md §4.3; "running"
// everywhere per §2.3).
type PlanStepStatus string

const (
	PlanStepPending   PlanStepStatus = "pending"
	PlanStepRunning   PlanStepStatus = "running"
	PlanStepCompleted PlanStepStatus = "completed"
	PlanStepFailed    PlanStepStatus = "failed"
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

// Item is one durable unit of work inside a run (API.md §4.3). A
// tag-discriminated union: Type selects which optional fields apply.
//
//	userMessage / agentMessage → Content
//	reasoning                  → Text, Redacted
//	plan                       → Steps
//	question                   → Question
//	toolCall                   → Tool, SafetyClass, Error
type Item struct {
	ID        string     `json:"id"`
	RunID     string     `json:"runId"`
	Status    ItemStatus `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	Type      ItemType   `json:"type"`

	Content     []ContentBlock  `json:"content,omitempty"`
	Text        string          `json:"text,omitempty"`
	Redacted    bool            `json:"redacted,omitempty"`
	Steps       []PlanStep      `json:"steps,omitempty"`
	Question    *Question       `json:"question,omitempty"`
	Tool        *ToolInvocation `json:"tool,omitempty"`
	SafetyClass SafetyClass     `json:"safetyClass,omitempty"`
	Error       *ProblemData    `json:"error,omitempty"` // tool-level failure (API.md §4.3)
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
	// Mime is the image media type ("image/png", "image/jpeg", …). Set on
	// image blocks; empty otherwise.
	Mime string `json:"mime,omitempty"`
	// Data is the base64-encoded image bytes (no "data:…;base64," prefix).
	// Set on image blocks; empty otherwise.
	Data string `json:"data,omitempty"`
}

// PlanStep is one step of a plan Item (API.md §4.3).
type PlanStep struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Status PlanStepStatus `json:"status"` // see PlanStepStatus
}

// Question is a structured clarifying question (API.md §4.3). answers
// (in AnswerResponse) are keyed by QuestionField.name.
type Question struct {
	Prompt string          `json:"prompt"`
	Fields []QuestionField `json:"fields"`
}

// QuestionField is one field of a Question. Type selects the shape:
//
//	text   → (no extra)
//	choice → Options, Multiple
type QuestionField struct {
	Name     string            `json:"name"`
	Label    string            `json:"label"`
	Header   string            `json:"header,omitempty"` // ≤12-char chip
	Required bool              `json:"required,omitempty"`
	Type     QuestionFieldType `json:"type"` // see QuestionFieldType
	Options  []QuestionOption  `json:"options,omitempty"`
	Multiple bool              `json:"multiple,omitempty"`
}

// QuestionOption is one choice option (API.md §4.3).
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

// NewToolInvocation constructs a ToolInvocation from raw JSON strings.
// argsJSON is the model's raw arguments (JSON-encoded); it is parsed into
// the structured Arguments map. outputJSON ("" before the tool completes)
// is decoded as best-effort JSON into Result — name-based display shaping
// (commandResult, searchHits, ...) lives in the server-side translator
// and is applied after construction. Arguments is always a non-nil map.
func NewToolInvocation(name, argsJSON, outputJSON string) *ToolInvocation {
	inv := &ToolInvocation{Name: name, Arguments: ParseArgs(argsJSON)}
	if inv.Arguments == nil {
		inv.Arguments = map[string]any{}
	}
	if outputJSON != "" {
		inv.Result = BestEffortJSON(outputJSON)
	}
	return inv
}

// ParseArgs decodes a tool call's JSON-encoded arguments into a
// structured map. Returns nil when empty or unparseable — callers
// normalize to the empty map {}.
func ParseArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

// BestEffortJSON decodes raw as JSON (object / array / scalar); when raw
// isn't valid JSON, the string is returned as-is. Used for tool results
// that don't have a richer display convention.
func BestEffortJSON(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return v
}

// ToolInvocation is the domain-neutral tool envelope (API.md §4.4). The
// core knows exactly ONE tool shape — not a union: Name is identity,
// Arguments is the parsed JSON object, Result is best-effort JSON output.
// "How a given tool is richly rendered" is domain knowledge that lives in
// the client's display registry keyed by Name (§4.4.2), never on the wire —
// so adding a tool costs zero protocol change (protocol-level OCP).
//
// Hard constraints (§4.4.1):
//   - Arguments is ALWAYS a JSON object, never a JSON string (no double
//     escaping). Streaming partial args arrive via ItemDelta.argumentsTextDelta
//     and are unmarshaled into Arguments at item.completed / the approval
//     payload (§4.8).
//   - Result is best-effort JSON, NEVER double-encoded; absent on
//     item.started, authoritative on item.completed (durable, §5.2). The
//     command-output preview rides ItemDelta.toolOutput, whose terminal value
//     is result.output (§5.2) — clients must not treat the streamed
//     accumulation as the source of truth.
//   - Tool-level failure does NOT go in Result — it rides the toolCall
//     Item's Error + status:"incomplete" (§4.3 / §8).
type ToolInvocation struct {
	Name      string         `json:"name"`             // tool identity (stable); MCP uses "<server>.<tool>"
	Arguments map[string]any `json:"arguments"`        // parsed JSON object (always present; never a JSON string)
	Result    any            `json:"result,omitempty"` // best-effort JSON; absent on item.started, authoritative on item.completed
}

// FileEdit is the applied result of one edit (API.md §4.5) — used in an
// edit/write tool's result {changes} (§4.4.2). status is past-tense (the
// post-change state); Diff is optional. No "untracked" (that's a VCS scan
// state only — see WorkspaceFileChange). Shares the FileEdit/WorkspaceFileChange
// status vocabulary deliberately (§4.5).
type FileEdit struct {
	Path   string     `json:"path"`
	Status FileStatus `json:"status"` // see FileStatus ("untracked" is VCS-only, never here)
	Diff   []DiffRow  `json:"diff,omitempty"`
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

// SearchHit is one LOCAL search hit (API.md §4.5) — used in a grep/glob
// tool's result {hits} (§4.4.2): grep = path+lineNumber+snippet, glob = path
// only. Distinct type from WebSearchResult: local (file+line) and web
// (url+title) are two mutually-exclusive shapes, never merged into one loose
// struct (which would let a result carry both path and url — an illegal but
// representable state).
type SearchHit struct {
	Path       string `json:"path"`
	LineNumber int    `json:"lineNumber,omitempty"`
	Snippet    string `json:"snippet,omitempty"`
}

// WebSearchResult is one web-search result (API.md §4.5) — used in a
// webSearch tool's result {results} (§4.4.2).
type WebSearchResult struct {
	Title      string `json:"title,omitempty"`
	URL        string `json:"url"`
	Snippet    string `json:"snippet,omitempty"`
	FaviconURL string `json:"faviconUrl,omitempty"`
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
