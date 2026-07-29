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
	// Duration is how long this segment executed before parking. A parked Run
	// still reports what it consumed, so the executor stamps it here for the same
	// reason it stamps it on TurnEnd.
	Duration time.Duration
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
	Reason execution.Outcome
	// Problem is present exactly when Reason is OutcomeError. It is already a
	// stable, client-safe application problem; executor diagnostics never enter
	// the event stream.
	Problem *transcript.Problem
	// Usage is the segment's final accounting, and is absent when the terminal
	// produced none — a cancellation or a failure joins without a TurnOutput to
	// account for. Absent is NOT zero: reading a missing report as "spent
	// nothing" made a canceled Run's committed metering fall back below what its
	// own progress events had already published.
	Usage    *TurnUsage
	Duration time.Duration
}

// TurnUsage is one authoritative accounting report for a segment. The three
// numbers are produced together by the executor's observer, so they travel
// together: a report that had tokens but no per-model split would be a different
// report, not this one with a field missing.
type TurnUsage struct {
	Tokens  accounting.TokenUsage
	ByModel []accounting.ModelUsage
	CostUSD float64
}

type UsageReported struct {
	engineEventBase
	TokenUsage    accounting.TokenUsage
	CostUSD       float64
	ContextTokens int64
}

// TodosUpdated reports the committed task list after a replacement — read back
// from the store, so what is published is what was written rather than what the
// tool was asked to write.
type TodosUpdated struct {
	engineEventBase
	State todo.State
}

type SteerMessage struct {
	engineEventBase
	Content []transcript.ContentBlock
}
