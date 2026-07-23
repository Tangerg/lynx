package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// TestEventLifecycleClassification locks the execution.Event contract the run
// pipeline switches on (the delivery projector reads it instead of re-deriving
// from the wire outcome): only runs.TurnEnd is a terminal — carrying its Outcome —
// only runs.TurnInterrupted parks, and every other event, including a pre-terminal
// runs.ErrorEvent, is a plain mid-run signal.
func TestEventLifecycleClassification(t *testing.T) {
	terminals := []struct {
		ev   execution.Event
		want execution.Outcome
	}{
		{runs.TurnEnd{Reason: execution.OutcomeCompleted}, execution.OutcomeCompleted},
		{runs.TurnEnd{Reason: execution.OutcomeError}, execution.OutcomeError},
		{runs.TurnEnd{Reason: execution.OutcomeMaxBudget}, execution.OutcomeMaxBudget},
		{runs.TurnEnd{Reason: execution.OutcomeMaxSteps}, execution.OutcomeMaxSteps},
		{runs.TurnEnd{Reason: execution.OutcomeCanceled}, execution.OutcomeCanceled},
	}
	for _, c := range terminals {
		o, ok := c.ev.Terminal()
		if !ok || o != c.want {
			t.Errorf("runs.TurnEnd{%v}.Terminal() = (%v, %v), want (%v, true)", c.want, o, ok, c.want)
		}
		if c.ev.Interrupt() {
			t.Errorf("runs.TurnEnd{%v}.Interrupt() = true, want false (a terminal is not a park)", c.want)
		}
	}

	if !(runs.TurnInterrupted{}).Interrupt() {
		t.Error("runs.TurnInterrupted.Interrupt() = false, want true")
	}
	if _, ok := (runs.TurnInterrupted{}).Terminal(); ok {
		t.Error("runs.TurnInterrupted.Terminal() ok = true, want false (a park is not a terminal)")
	}

	// A representative sample of mid-run events: neither terminal nor park. An
	// runs.ErrorEvent is deliberately here — it is a pre-terminal record; the runs.TurnEnd
	// that follows carries OutcomeError.
	midRun := []execution.Event{
		runs.TurnStart{}, runs.MessageDelta{}, runs.ReasoningDelta{}, runs.ToolCallStart{}, runs.ToolCallEnd{},
		runs.UsageReported{}, runs.CompactBoundary{}, runs.TodosUpdated{}, runs.SteerMessage{}, runs.ErrorEvent{},
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
