package protocol

import (
	"context"
	"time"
)

// Items is the items.* method group — durable conversation history
// (API.md §7.4). History = the completed Item sequence; there is no
// separate Message type.
type Items interface {
	// ListItems returns completed items plus the RunRef records needed
	// to reconstruct their run tree + continuation chain (API.md §7.4).
	ListItems(ctx context.Context, in ListItemsRequest) (*ListItemsResponse, error)
	// EditItem edits an item and starts a continuation Run (resume
	// semantics). Gated on features.checkpoints.
	EditItem(ctx context.Context, in EditItemRequest) (*EditItemResponse, error)
}

// ListItemsRequest — items.list body.
type ListItemsRequest struct {
	SessionID string `json:"sessionId"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
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

// EditItemRequest — items.edit body.
type EditItemRequest struct {
	ItemID      string         `json:"itemId"`
	Replacement []ContentBlock `json:"replacement"`
}

// EditItemResponse — items.edit result; a continuation Run.
type EditItemResponse struct {
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId"`
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
	SafetyClass string          `json:"safetyClass,omitempty"`
	Error       *ProblemData    `json:"error,omitempty"` // tool-level failure (API.md §4.3)
}

// ContentBlock is one block of message content (API.md §4.3).
//
//	text  → Text
//	image → AttachmentID
type ContentBlock struct {
	Type         string `json:"type"` // "text" | "image"
	Text         string `json:"text,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
}

// PlanStep is one step of a plan Item (API.md §4.3).
type PlanStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // "pending" | "running" | "completed" | "failed"
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
	Name     string           `json:"name"`
	Label    string           `json:"label"`
	Header   string           `json:"header,omitempty"` // ≤12-char chip
	Required bool             `json:"required,omitempty"`
	Type     string           `json:"type"` // "text" | "choice"
	Options  []QuestionOption `json:"options,omitempty"`
	Multiple bool             `json:"multiple,omitempty"`
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
	Path   string    `json:"path"`
	Status string    `json:"status"` // "added" | "modified" | "deleted" | "renamed"
	Diff   []DiffRow `json:"diff,omitempty"`
}

// DiffRow is one structured row of a unified diff (API.md §4.5). Code
// is plain text — the client highlights.
//
//	hunk    → Text
//	context → LeftLine, RightLine, Code
//	added   → RightLine, Code
//	deleted → LeftLine, Code
type DiffRow struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	LeftLine  int    `json:"leftLine,omitempty"`
	RightLine int    `json:"rightLine,omitempty"`
	Code      string `json:"code,omitempty"`
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

// Usage is cumulative token usage (API.md §4.6).
type Usage struct {
	InputTokens      int64                 `json:"inputTokens,omitempty"`
	OutputTokens     int64                 `json:"outputTokens,omitempty"`
	ReasoningTokens  int64                 `json:"reasoningTokens,omitempty"`
	CacheReadTokens  int64                 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64                 `json:"cacheWriteTokens,omitempty"`
	ByModel          map[string]ModelUsage `json:"byModel,omitempty"`
}

// ModelUsage is one model's slice of usage (API.md §4.6).
type ModelUsage struct {
	InputTokens     int64    `json:"inputTokens,omitempty"`
	OutputTokens    int64    `json:"outputTokens,omitempty"`
	ReasoningTokens int64    `json:"reasoningTokens,omitempty"`
	CostUSD         *float64 `json:"costUsd,omitempty"`
}
