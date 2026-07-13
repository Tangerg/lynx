package transcript

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type Run struct {
	SessionID       string
	ID              string
	SpawnedByItemID string
	Provider        string
	Model           string
	State           execution.RunState
	Outcome         *execution.Outcome
	Result          *RunResult
	Detail          string
	Interrupts      []Interrupt
	CreatedAt       time.Time
	FinishedAt      time.Time
	UpdatedAt       time.Time
	MessageMark     int
}

type RunResult struct {
	Usage    *Usage
	Steps    int
	Error    *Problem
	Duration time.Duration
}

type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

type Usage struct {
	ModelUsage
	ByModel map[string]ModelUsage
}

type ItemStatus uint8

const (
	ItemRunning ItemStatus = iota
	ItemCompleted
	ItemIncomplete
)

type ItemKind uint8

const (
	UserMessage ItemKind = iota
	AgentMessage
	Reasoning
	Plan
	QuestionItem
	ToolCall
	Compaction
)

type Item struct {
	SessionID string
	ID        string
	RunID     string
	Status    ItemStatus
	CreatedAt time.Time
	Kind      ItemKind

	Content     []ContentBlock
	Text        string
	Redacted    bool
	Steps       []PlanStep
	Question    *Question
	Tool        *ToolInvocation
	SafetyClass string
	Error       *Problem

	Summary         string
	DroppedMessages int
}

type ContentKind uint8

const (
	TextContent ContentKind = iota
	ImageContent
)

type ContentBlock struct {
	Kind ContentKind
	Text string
	Mime string
	Data string
}

type PlanStep struct {
	ID     string
	Title  string
	Status string
}

type Question struct {
	Prompt string
	Fields []QuestionField
}

type QuestionField struct {
	Name     string
	Label    string
	Header   string
	Required bool
	Kind     QuestionFieldKind
	Options  []QuestionOption
	Multiple bool
}

type QuestionFieldKind uint8

const (
	QuestionText QuestionFieldKind = iota
	QuestionChoice
)

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

type ToolInvocation struct {
	Name      string
	Arguments map[string]any
	Result    *ToolResult
}

type ToolResultKind uint8

const (
	RawToolResult ToolResultKind = iota
	CommandToolResult
	SearchToolResult
	WebSearchToolResult
	FileChangeToolResult
)

type ToolResult struct {
	Kind       ToolResultKind
	Raw        any
	Command    *CommandResult
	Search     *SearchResult
	WebSearch  *WebSearchResultSet
	FileChange *FileChangeResult
}

type ProblemScope uint8

const (
	RunProblem ProblemScope = iota
	ToolProblem
)

type ProblemKind uint8

const (
	InternalProblem ProblemKind = iota
	AgentStuckProblem
	RateLimitedProblem
	InvalidAPIKeyProblem
	TimeoutProblem
	ProviderUnavailableProblem
	ProviderRejectedProblem
	DeniedByUserProblem
	ToolFailedProblem
)

type Problem struct {
	Kind              ProblemKind
	Scope             ProblemScope
	Detail            string
	DocURL            string
	Retryable         bool
	RetryAfterSeconds int
}

type InterruptKind uint8

const (
	ApprovalInterrupt InterruptKind = iota
	QuestionInterrupt
)

type Interrupt struct {
	ItemID   string
	Kind     InterruptKind
	Approval *Approval
	Question *Question
}

type Approval struct {
	Tool   ToolInvocation
	Risk   string
	Reason string
}

type CommandResult struct {
	ExitCode        *int
	Output          string
	OutputTruncated bool
}

type SearchResult struct{ Hits []SearchHit }
type WebSearchResultSet struct{ Results []WebSearchResult }
type FileChangeResult struct{ Changes []FileEdit }

type SearchHit struct {
	Path       string
	LineNumber int
	Snippet    string
}

type WebSearchResult struct {
	Title      string
	URL        string
	Snippet    string
	FaviconURL string
}

type FileEdit struct {
	Path   string
	Status string
	Diff   []DiffRow
}

type DiffRowKind uint8

const (
	DiffHunk DiffRowKind = iota
	DiffContext
	DiffAdded
	DiffDeleted
)

type DiffRow struct {
	Kind      DiffRowKind
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}
