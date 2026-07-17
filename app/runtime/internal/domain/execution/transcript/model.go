package transcript

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ErrIdentityConflict reports an attempt to reuse a durable transcript identity
// for a different owner. Item ids are bound to one Session+Run and Run ids to
// one Session for their entire lifetime; persistence must never re-parent them.
var ErrIdentityConflict = errors.New("transcript: identity conflict")

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
	SafetyClass tool.SafetyClass
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
	Result    any
}

type ProblemScope uint8

const (
	RunProblem ProblemScope = iota
	ToolProblem
)

type ProblemKind uint8

const (
	InternalProblem ProblemKind = iota
	RunLostProblem
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
	Risk   tool.RiskLevel
	Reason string
}
