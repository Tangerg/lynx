package runs

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// EngineEvent is the application-owned execution event sum type. Driven
// adapters emit these values at the runs.SegmentExecutor port; delivery therefore
// projects a stable application contract and never reaches back into an
// executor adapter. The unexported marker seals the family to this package.
type EngineEvent interface {
	execution.Event
	WithMeta(EventMeta) EngineEvent
	engineEvent()
}

// EventMeta is correlation metadata supplied by the executor adapter. Run event
// replay uses the Coordinator's own cursor; this metadata is diagnostic only.
type EventMeta struct {
	SessionID string
	TurnID    string
	Seq       uint64
	Timestamp time.Time
}

func (EventMeta) engineEvent()                        {}
func (EventMeta) Terminal() (execution.Outcome, bool) { return 0, false }
func (EventMeta) Interrupt() bool                     { return false }

type TurnStart struct {
	EventMeta
	Model string
}

type MessageDelta struct {
	EventMeta
	Text string
}

type ReasoningDelta struct {
	EventMeta
	Text string
}

type ToolCallStart struct {
	EventMeta
	CallID      string
	ToolName    string
	Arguments   string
	SafetyClass tool.SafetyClass
}

type ToolCallEnd struct {
	EventMeta
	CallID       string
	Arguments    string
	Result       *tool.Result
	Offload      *offload.Ref
	OutputText   string
	MutatedPaths []string
	Err          string
	Denied       bool
}

type CompactBoundary struct {
	EventMeta
	MessagesBefore int
	MessagesAfter  int
}

type TurnInterrupted struct {
	EventMeta
	Interrupts []Interrupt
}

func (TurnInterrupted) Interrupt() bool { return true }

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
	EventMeta
	Reason       execution.Outcome
	TokenUsage   accounting.TokenUsage
	UsageByModel []accounting.ModelUsage
	CostUSD      float64
	Duration     time.Duration
	MaxBudget    int64
	MaxCostUSD   float64
	MaxSteps     int
}

func (e TurnEnd) Terminal() (execution.Outcome, bool) { return e.Reason, true }

type ErrorEvent struct {
	EventMeta
	Message string
	Code    string
}

type UsageReported struct {
	EventMeta
	TokenUsage    accounting.TokenUsage
	CostUSD       float64
	ContextTokens int64
}

type TodosUpdated struct {
	EventMeta
	Todos []todo.Item
}

type SteerMessage struct {
	EventMeta
	Text string
}

func (e TurnStart) WithMeta(m EventMeta) EngineEvent       { e.EventMeta = m; return e }
func (e MessageDelta) WithMeta(m EventMeta) EngineEvent    { e.EventMeta = m; return e }
func (e ReasoningDelta) WithMeta(m EventMeta) EngineEvent  { e.EventMeta = m; return e }
func (e ToolCallStart) WithMeta(m EventMeta) EngineEvent   { e.EventMeta = m; return e }
func (e ToolCallEnd) WithMeta(m EventMeta) EngineEvent     { e.EventMeta = m; return e }
func (e CompactBoundary) WithMeta(m EventMeta) EngineEvent { e.EventMeta = m; return e }
func (e TurnInterrupted) WithMeta(m EventMeta) EngineEvent { e.EventMeta = m; return e }
func (e TurnEnd) WithMeta(m EventMeta) EngineEvent         { e.EventMeta = m; return e }
func (e ErrorEvent) WithMeta(m EventMeta) EngineEvent      { e.EventMeta = m; return e }
func (e UsageReported) WithMeta(m EventMeta) EngineEvent   { e.EventMeta = m; return e }
func (e TodosUpdated) WithMeta(m EventMeta) EngineEvent    { e.EventMeta = m; return e }
func (e SteerMessage) WithMeta(m EventMeta) EngineEvent    { e.EventMeta = m; return e }
