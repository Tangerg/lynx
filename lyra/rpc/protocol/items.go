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

// ListItemsResponse — items.list result. Flat items array (avoids the
// awkward resp.items.data). runs carries Run-level structure (API.md §10.3).
type ListItemsResponse struct {
	Items      []Item   `json:"items"`
	NextCursor string   `json:"nextCursor,omitempty"`
	Runs       []RunRef `json:"runs"`
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
	ItemStatusInProgress ItemStatus = "inProgress"
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
	Status string `json:"status"` // "pending" | "inProgress" | "completed" | "failed"
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

// ToolInvocationKind discriminates the ToolInvocation union (API.md §4.4).
type ToolInvocationKind string

const (
	ToolKindCommand  ToolInvocationKind = "command"
	ToolKindFileEdit ToolInvocationKind = "fileEdit"
	ToolKindMCP      ToolInvocationKind = "mcp"
	ToolKindSearch   ToolInvocationKind = "search"
	ToolKindSubagent ToolInvocationKind = "subagent"
)

// ToolInvocation is a tag-discriminated union over tool kinds (API.md §4.4).
// Completed-item Arguments is the parsed structured value; streaming
// partial args arrive via ItemDelta.argumentsTextDelta (API.md §5.1).
//
//	command  → Command, Cwd, Output, ExitCode
//	fileEdit → Path, Diff
//	mcp      → Server, Name, Arguments, Result
//	search   → Query, Results
//	subagent → Name, Prompt, ChildRunID, Result
type ToolInvocation struct {
	Kind ToolInvocationKind `json:"kind"`

	Command   string         `json:"command,omitempty"`
	Cwd       string         `json:"cwd,omitempty"`
	Output    string         `json:"output,omitempty"`
	ExitCode  *int           `json:"exitCode,omitempty"`
	Path      string         `json:"path,omitempty"`
	Diff      []DiffRow      `json:"diff,omitempty"`
	Server    string         `json:"server,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    any            `json:"result,omitempty"`
	Query     string         `json:"query,omitempty"`
	Results   []SearchResult `json:"results,omitempty"`
	// subagent
	Prompt     []ContentBlock `json:"prompt,omitempty"`
	ChildRunID string         `json:"childRunId,omitempty"`
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

// SearchResult is one search hit (API.md §4.5).
type SearchResult struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Path    string `json:"path,omitempty"`
	Snippet string `json:"snippet,omitempty"`
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
