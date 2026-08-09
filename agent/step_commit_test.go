package agent

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPreparedStepFinalizationCountsEveryImmediateChildSignal(t *testing.T) {
	limits := Limits{
		MaxSteps: 10, MaxEffects: 10, MaxSignals: 3, MaxPendingSignals: 10,
	}
	loop := &processLoop{
		limits: limits,
		budget: budgetFromLimits(limits),
	}
	mailbox := newSignalMailbox()
	firstWait, _ := ParseWaitID("wait:first")
	secondWait, _ := ParseWaitID("wait:second")
	firstKey, _ := ParseWaitKey("first")
	secondKey, _ := ParseWaitKey("second")
	if err := mailbox.registerWait(firstKey, firstWait, false); err != nil {
		t.Fatal(err)
	}
	if err := mailbox.registerWait(secondKey, secondWait, false); err != nil {
		t.Fatal(err)
	}
	firstSignalID, _ := ParseSignalID("signal:first")
	secondSignalID, _ := ParseSignalID("signal:second")
	firstSignal, _ := newSignal(firstSignalID, firstWait, time.Now(), json.RawMessage(`{}`))
	secondSignal, _ := newSignal(secondSignalID, secondWait, time.Now(), json.RawMessage(`{}`))
	finalization := &preparedStepFinalization{
		loop: loop,
		prepared: &preparedStep{wire: preparedStepWire{
			Effects: make([]preparedEffectWire, 2),
		}},
		mailbox:               mailbox,
		immediateChildSignals: []Signal{firstSignal, secondSignal},
	}

	err := finalization.enqueueImmediateChildSignals()
	if !errors.Is(err, ErrResourceLimitExceeded) {
		t.Fatalf("enqueue immediate child Signals error = %v, want %v", err, ErrResourceLimitExceeded)
	}
	if pending := finalization.mailbox.pendingCount(); pending != 1 {
		t.Fatalf("pending immediate child Signals = %d, want 1 before cumulative limit", pending)
	}
}
