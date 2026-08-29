package agent

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestStepCannotConsumeBudgetReservedAtUint64Boundary(t *testing.T) {
	const maxUint64 = ^uint64(0)
	processID, err := ParseProcessID("process:resource-boundary")
	if err != nil {
		t.Fatal(err)
	}
	controller := newProcessController(
		rootProcessRelation(processID), DeploymentRef{},
		Budget{Steps: maxUint64, Effects: maxUint64, Signals: maxUint64},
		CapabilitySet{}, DefaultTreeLimits(), time.Now(), StatusRunning,
	)
	loop := &processState{
		engine:         &Engine{},
		controller:     controller,
		status:         StatusRunning,
		committedSteps: maxUint64 - 1,
		limits: Limits{
			MaxSteps: maxUint64, MaxEffects: maxUint64,
			MaxSignals: maxUint64, MaxPendingSignals: maxUint64,
		},
		budget:         Budget{Steps: maxUint64, Effects: maxUint64, Signals: maxUint64},
		reservedBudget: Budget{Steps: 1, Effects: 1, Signals: 1},
		mailbox:        newSignalMailbox(),
	}

	schedulingFailure := loop.stepSchedulingFailure()
	if schedulingFailure == nil {
		t.Fatal("step scheduling failure is nil")
	}
	loop.fail(schedulingFailure.kind, schedulingFailure.code, schedulingFailure.cause)

	if loop.status != StatusFailed {
		t.Fatalf("status = %s, want %s", loop.status, StatusFailed)
	}
	failure, present := loop.termination.Failure()
	if !present || failure.Code() != "engine.limit.steps" {
		t.Fatalf("failure = %+v, present = %t", failure, present)
	}
}

func TestPreparedStepFinalizationCountsEveryImmediateChildSignal(t *testing.T) {
	limits := Limits{
		MaxSteps: 10, MaxEffects: 10, MaxSignals: 3, MaxPendingSignals: 10,
	}
	loop := &processState{
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
