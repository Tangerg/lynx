package runs

import "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"

type Item = transcript.Item
type ItemStatus = transcript.ItemStatus
type ContentBlock = transcript.ContentBlock
type PlanStep = transcript.PlanStep
type Question = transcript.Question
type QuestionField = transcript.QuestionField
type QuestionOption = transcript.QuestionOption
type ToolInvocation = transcript.ToolInvocation
type Problem = transcript.Problem
type ProblemKind = transcript.ProblemKind
type ProblemScope = transcript.ProblemScope
type RunResult = transcript.RunResult
type ModelUsage = transcript.ModelUsage
type Usage = transcript.Usage

const (
	ItemRunning                = transcript.ItemRunning
	ItemSucceeded              = transcript.ItemCompleted
	ItemIncomplete             = transcript.ItemIncomplete
	UserMessage                = transcript.UserMessage
	AgentMessage               = transcript.AgentMessage
	Reasoning                  = transcript.Reasoning
	Plan                       = transcript.Plan
	QuestionItem               = transcript.QuestionItem
	ToolCall                   = transcript.ToolCall
	Compaction                 = transcript.Compaction
	TextContent                = transcript.TextContent
	ImageContent               = transcript.ImageContent
	QuestionText               = transcript.QuestionText
	QuestionChoice             = transcript.QuestionChoice
	RunProblem                 = transcript.RunProblem
	ToolProblem                = transcript.ToolProblem
	InternalProblem            = transcript.InternalProblem
	AgentStuckProblem          = transcript.AgentStuckProblem
	RateLimitedProblem         = transcript.RateLimitedProblem
	InvalidAPIKeyProblem       = transcript.InvalidAPIKeyProblem
	TimeoutProblem             = transcript.TimeoutProblem
	ProviderUnavailableProblem = transcript.ProviderUnavailableProblem
	ProviderRejectedProblem    = transcript.ProviderRejectedProblem
	DeniedByUserProblem        = transcript.DeniedByUserProblem
	ToolFailedProblem          = transcript.ToolFailedProblem
	ApprovalInterrupt          = transcript.ApprovalInterrupt
	QuestionInterrupt          = transcript.QuestionInterrupt
)

type RunEvent interface {
	runEvent()
	Durable() bool
	Terminal() bool
}

type SegmentStarted struct{ Run transcript.Run }
type SegmentProgressed struct{ Progress RunProgress }
type SegmentFinished struct{ Run transcript.Run }
type ItemStarted struct{ Item transcript.Item }
type ItemChanged struct {
	ItemID string
	Delta  ItemDelta
}
type ItemCompleted struct {
	Item         transcript.Item
	mutatedPaths []string
}
type StateSnapshot struct{ Todos []TodoSnapshot }

func (SegmentStarted) runEvent()    {}
func (SegmentProgressed) runEvent() {}
func (SegmentFinished) runEvent()   {}
func (ItemStarted) runEvent()       {}
func (ItemChanged) runEvent()       {}
func (ItemCompleted) runEvent()     {}
func (StateSnapshot) runEvent()     {}

func (SegmentStarted) Durable() bool    { return true }
func (SegmentProgressed) Durable() bool { return false }
func (SegmentFinished) Durable() bool   { return true }
func (ItemStarted) Durable() bool       { return true }
func (ItemChanged) Durable() bool       { return false }
func (ItemCompleted) Durable() bool     { return true }
func (StateSnapshot) Durable() bool     { return true }

func (SegmentStarted) Terminal() bool    { return false }
func (SegmentProgressed) Terminal() bool { return false }
func (SegmentFinished) Terminal() bool   { return true }
func (ItemStarted) Terminal() bool       { return false }
func (ItemChanged) Terminal() bool       { return false }
func (ItemCompleted) Terminal() bool     { return false }
func (StateSnapshot) Terminal() bool     { return false }

type RunProgress struct {
	Step          *int
	Usage         *transcript.Usage
	ContextTokens *int64
	ToolName      string
}

type ItemDeltaKind uint8

const (
	ContentDelta ItemDeltaKind = iota
	ReasoningDeltaKind
	ToolArgumentsDelta
	ToolOutputDelta
	PlanDelta
)

type ItemDelta struct {
	Kind               ItemDeltaKind
	Index              *int
	Text               string
	ArgumentsTextDelta string
	Steps              []transcript.PlanStep
}

type TodoSnapshot struct {
	ID            string
	Text          string
	Status        string
	BlockedReason string
	NextAction    string
}
