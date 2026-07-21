package turn

import "github.com/Tangerg/lynx/app/runtime/internal/application/runs"

// The application owns the canonical execution event vocabulary. The turn
// adapter emits those values directly; aliases preserve a concise native API
// without maintaining a second, lockstep event family.
type Event = runs.EngineEvent
type EventMeta = runs.EventMeta
type TurnStart = runs.TurnStart
type MessageDelta = runs.MessageDelta
type ReasoningDelta = runs.ReasoningDelta
type ToolCallStart = runs.ToolCallStart
type ToolCallEnd = runs.ToolCallEnd
type CompactBoundary = runs.CompactBoundary
type TurnInterrupted = runs.TurnInterrupted
type Interrupt = runs.Interrupt
type InterruptKind = runs.InterruptKind
type ApprovalPrompt = runs.ApprovalPrompt
type TurnEnd = runs.TurnEnd
type ErrorCode = runs.ErrorCode
type ErrorEvent = runs.ErrorEvent
type UsageReported = runs.UsageReported
type TodosUpdated = runs.TodosUpdated
type SteerMessage = runs.SteerMessage

const (
	ApprovalInterruptKind      = runs.ApprovalInterruptKind
	QuestionInterruptKind      = runs.QuestionInterruptKind
	ErrorCodeEngine            = runs.ErrorCodeEngine
	ErrorCodeAgentStuck        = runs.ErrorCodeAgentStuck
	ErrorCodeModelUnavailable  = runs.ErrorCodeModelUnavailable
	ErrorCodeSteering          = runs.ErrorCodeSteering
	ErrorCodeCompaction        = runs.ErrorCodeCompaction
	ErrorCodeMemoryMaintenance = runs.ErrorCodeMemoryMaintenance
	ErrorCodeSkillMaintenance  = runs.ErrorCodeSkillMaintenance
)
