package runs

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// EngineEvent is the closed application-owned execution event family. Driven
// adapters emit these values at the SegmentExecutor port; delivery therefore
// projects an application contract and never reaches into an executor adapter.
type EngineEvent interface {
	engineEvent()
}

type engineEventBase struct{}

func (engineEventBase) engineEvent() {}

type TurnStart struct {
	engineEventBase
	Model string
}

type MessageDelta struct {
	engineEventBase
	Text string
}

type ReasoningDelta struct {
	engineEventBase
	Text string
}

type ToolCallStart struct {
	engineEventBase
	CallID      string
	ToolName    string
	Arguments   string
	Activity    string
	SafetyClass tool.SafetyClass
}

type ToolCallEnd struct {
	engineEventBase
	CallID       string
	Arguments    string
	Result       *tool.Result
	Offload      *offload.Ref
	OutputText   string
	MutatedPaths []string
	Err          string
	Denied       bool
}

// FileChange is a live workspace refresh nudge emitted after a tool-owned file
// mutation commits. Delivery only encodes these already-resolved values.
type FileChange struct {
	Cwd   string
	Paths []string
}

type CompactBoundary struct {
	engineEventBase
	MessagesBefore int
	MessagesAfter  int
}

type TurnInterrupted struct {
	engineEventBase
	Interrupts []Interrupt
}

func (e TurnInterrupted) validate() error {
	if len(e.Interrupts) == 0 {
		return errors.New("runs: executor emitted an empty interrupt")
	}
	for _, interrupt := range e.Interrupts {
		if err := interrupt.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type TurnEnd struct {
	engineEventBase
	Reason       execution.Outcome
	TokenUsage   accounting.TokenUsage
	UsageByModel []accounting.ModelUsage
	CostUSD      float64
	Duration     time.Duration
	MaxBudget    int64
	MaxCostUSD   float64
	MaxSteps     int
}

// ErrorCode identifies the operation that reported an executor error. It is a
// typed diagnostic tag; user-facing classification is carried by Problem.
type ErrorCode string

const (
	ErrorCodeEngine            ErrorCode = "ENGINE_ERROR"
	ErrorCodeAgentStuck        ErrorCode = "AGENT_STUCK"
	ErrorCodeModelUnavailable  ErrorCode = "MODEL_UNAVAILABLE"
	ErrorCodeSteering          ErrorCode = "STEERING_ERROR"
	ErrorCodeCompaction        ErrorCode = "COMPACTION_ERROR"
	ErrorCodeMemoryMaintenance ErrorCode = "MEMORY_MAINTENANCE_ERROR"
	ErrorCodeSkillMaintenance  ErrorCode = "SKILL_MAINTENANCE_ERROR"
)

type ErrorEvent struct {
	engineEventBase
	Message string
	Code    ErrorCode
	Problem transcript.Problem
}

type UsageReported struct {
	engineEventBase
	TokenUsage    accounting.TokenUsage
	CostUSD       float64
	ContextTokens int64
}

type TodosUpdated struct {
	engineEventBase
	Todos []todo.Item
}

type SteerMessage struct {
	engineEventBase
	Text string
}
