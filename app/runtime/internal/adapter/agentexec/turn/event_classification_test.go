package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// TestEventLifecycleClassification locks the execution.Event contract the run
// pipeline switches on (the delivery projector reads it instead of re-deriving
// from the wire outcome): only TurnEnd is a terminal — carrying its Outcome —
// only TurnInterrupted parks, and every other event, including a pre-terminal
// ErrorEvent, is a plain mid-run signal.
func TestEventLifecycleClassification(t *testing.T) {
	terminals := []struct {
		ev   execution.Event
		want execution.Outcome
	}{
		{TurnEnd{Reason: execution.OutcomeCompleted}, execution.OutcomeCompleted},
		{TurnEnd{Reason: execution.OutcomeError}, execution.OutcomeError},
		{TurnEnd{Reason: execution.OutcomeMaxBudget}, execution.OutcomeMaxBudget},
		{TurnEnd{Reason: execution.OutcomeMaxSteps}, execution.OutcomeMaxSteps},
		{TurnEnd{Reason: execution.OutcomeCanceled}, execution.OutcomeCanceled},
	}
	for _, c := range terminals {
		o, ok := c.ev.Terminal()
		if !ok || o != c.want {
			t.Errorf("TurnEnd{%v}.Terminal() = (%v, %v), want (%v, true)", c.want, o, ok, c.want)
		}
		if c.ev.Interrupt() {
			t.Errorf("TurnEnd{%v}.Interrupt() = true, want false (a terminal is not a park)", c.want)
		}
	}

	if !(TurnInterrupted{}).Interrupt() {
		t.Error("TurnInterrupted.Interrupt() = false, want true")
	}
	if _, ok := (TurnInterrupted{}).Terminal(); ok {
		t.Error("TurnInterrupted.Terminal() ok = true, want false (a park is not a terminal)")
	}

	// A representative sample of mid-run events: neither terminal nor park. An
	// ErrorEvent is deliberately here — it is a pre-terminal record; the TurnEnd
	// that follows carries OutcomeError.
	midRun := []execution.Event{
		TurnStart{}, MessageDelta{}, ReasoningDelta{}, ToolCallStart{}, ToolCallEnd{},
		UsageReported{}, CompactBoundary{}, MemoryUpdated{}, TodosUpdated{}, SteerMessage{}, ErrorEvent{},
	}
	for _, ev := range midRun {
		if _, ok := ev.Terminal(); ok {
			t.Errorf("%T.Terminal() ok = true, want false", ev)
		}
		if ev.Interrupt() {
			t.Errorf("%T.Interrupt() = true, want false", ev)
		}
	}
}
