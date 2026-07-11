package sessions

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestCancelParkedRunCancelsTurnBeforeDeletingInterrupt(t *testing.T) {
	var order []string
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
			onDelete: func(string) { order = append(order, "delete") },
		},
	}
	turns := stubTurns{onCancel: func(h turn.TurnHandle) {
		order = append(order, "cancel")
		if h.SessionID != "ses_1" || h.TurnID != "turn_1" {
			t.Fatalf("handle = %+v, want ses_1/turn_1", h)
		}
	}}

	if err := newCoordinator(stores, turns).CancelParkedRun(context.Background(), "run_1"); err != nil {
		t.Fatalf("cancel parked run: %v", err)
	}
	if got := stores.interrupts.deleted; len(got) != 1 || got[0] != "run_1" {
		t.Fatalf("deleted = %v, want [run_1]", got)
	}
	if len(order) != 2 || order[0] != "cancel" || order[1] != "delete" {
		t.Fatalf("order = %v, want cancel then delete", order)
	}
}

func TestCancelParkedRunMissing(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	err := newCoordinator(stores, stubTurns{}).CancelParkedRun(context.Background(), "missing")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
	if len(stores.interrupts.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", stores.interrupts.deleted)
	}
}

func TestClaimRunSlotHoldsAndReleasesSession(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	admission, err := newCoordinator(stores, nil).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if err != nil {
		t.Fatalf("claim run slot: %v", err)
	}
	if admission.SessionID != "ses_1" {
		t.Fatalf("admission session = %q, want ses_1", admission.SessionID)
	}
	if !claimer.claimed["ses_1"] {
		t.Fatal("session should be claimed")
	}

	admission.Release()
	copyOfAdmission := admission
	copyOfAdmission.Release()
	if claimer.claimed["ses_1"] {
		t.Fatal("session should be released")
	}
	if len(claimer.released) != 1 || claimer.released[0] != "ses_1" {
		t.Fatalf("released = %v, want [ses_1]", claimer.released)
	}
}

func TestClaimRunSlotRejectsOpenInterrupt(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	_, err := newCoordinator(stores, nil).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("err = %v, want ErrSessionBusy", err)
	}
	if claimer.claimed["ses_1"] {
		t.Fatal("failed admission must release its claim")
	}
	if len(claimer.released) != 1 || claimer.released[0] != "ses_1" {
		t.Fatalf("released = %v, want [ses_1]", claimer.released)
	}
}

func TestClaimRunSlotRejectsActiveClaim(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{claimed: map[string]bool{"ses_1": true}}

	_, err := newCoordinator(stores, nil).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("err = %v, want ErrSessionBusy", err)
	}
	if len(claimer.released) != 0 {
		t.Fatalf("released = %v, want none", claimer.released)
	}
}

func TestClaimMutationSlotAllowsOpenInterrupt(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	admission, err := newCoordinator(stores, nil).ClaimMutationSlot(claimer, "ses_1")
	if err != nil {
		t.Fatalf("claim mutation slot: %v", err)
	}
	if admission.SessionID != "ses_1" || !claimer.claimed["ses_1"] {
		t.Fatalf("admission = %+v claimed = %v, want ses_1 claimed", admission, claimer.claimed)
	}
	admission.Release()
}

func TestClaimResumeSlotPeeksAndClaimsInterruptSession(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	claimer := &testClaimer{}

	pending, admission, err := newCoordinator(stores, nil).ClaimResumeSlot(context.Background(), claimer, "run_1")
	if err != nil {
		t.Fatalf("claim resume slot: %v", err)
	}
	if pending.ParentRunID != "run_1" || pending.SessionID != "ses_1" {
		t.Fatalf("pending = %+v, want run_1/ses_1", pending)
	}
	if admission.SessionID != "ses_1" || !claimer.claimed["ses_1"] {
		t.Fatalf("admission = %+v claimed = %v, want ses_1 claimed", admission, claimer.claimed)
	}

	admission.Release()
	if claimer.claimed["ses_1"] {
		t.Fatal("resume admission should release the session")
	}
}

func TestClaimResumeSlotMissingInterrupt(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	_, _, err := newCoordinator(stores, nil).ClaimResumeSlot(context.Background(), claimer, "missing")
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("err = %v, want ErrInterruptNotOpen", err)
	}
	if len(claimer.claimed) != 0 {
		t.Fatalf("claimed = %v, want none", claimer.claimed)
	}
}

func TestResumeClaimedInterruptConsumesAndResumes(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	resolution := interrupts.Resolution{Approved: true}
	turns := stubTurns{onResume: func(h turn.TurnHandle, got interrupts.Resolution, interruptKinds []string) {
		if h.SessionID != "ses_1" || h.TurnID != "turn_1" {
			t.Fatalf("handle = %+v, want ses_1/turn_1", h)
		}
		if !got.Approved {
			t.Fatalf("resolution = %+v, want approved", got)
		}
		if len(interruptKinds) != 1 || interruptKinds[0] != "approval" {
			t.Fatalf("interrupt kinds = %+v, want approval", interruptKinds)
		}
	}}

	resumed, err := newCoordinator(stores, turns).ResumeClaimedInterrupt(context.Background(), "run_1", resolution, []string{"approval"})
	if err != nil {
		t.Fatalf("resume claimed interrupt: %v", err)
	}
	if resumed.Pending.ParentRunID != "run_1" || resumed.Handle.TurnID != "turn_1" {
		t.Fatalf("resumed = %+v", resumed)
	}
	if _, ok := stores.interrupts.pending["run_1"]; ok {
		t.Fatal("interrupt must be consumed")
	}
}

func TestResumeClaimedInterruptRehydratesMissingTurn(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {
					ParentRunID: "run_1",
					SessionID:   "ses_1",
					TurnID:      "turn_1",
					ProcessID:   "proc_1",
					Provider:    "anthropic",
					Model:       "claude",
				},
			},
		},
	}
	turns := stubTurns{
		resumeErr:       turn.ErrTurnNotFound,
		rehydrateHandle: turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_rebuilt"},
		onRehydrate: func(req turn.RehydrateRequest) {
			if req.SessionID != "ses_1" || req.ProcessID != "proc_1" || !req.Approved || req.Provider != "anthropic" || req.Model != "claude" {
				t.Fatalf("rehydrate request = %+v", req)
			}
			if len(req.InterruptKinds) != 1 || req.InterruptKinds[0] != "approval" {
				t.Fatalf("interrupt kinds = %+v, want approval", req.InterruptKinds)
			}
		},
	}

	resumed, err := newCoordinator(stores, turns).ResumeClaimedInterrupt(context.Background(), "run_1", interrupts.Resolution{Approved: true}, []string{"approval"})
	if err != nil {
		t.Fatalf("resume claimed interrupt: %v", err)
	}
	if resumed.Handle.TurnID != "turn_rebuilt" {
		t.Fatalf("handle = %+v, want rebuilt turn", resumed.Handle)
	}
}

func TestResumeClaimedInterruptParkClaimed(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	turns := stubTurns{resumeErr: turn.ErrParkClaimed}

	_, err := newCoordinator(stores, turns).ResumeClaimedInterrupt(context.Background(), "run_1", interrupts.Resolution{Approved: true}, nil)
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("err = %v, want ErrInterruptNotOpen", err)
	}
}

func TestResumeClaimedInterruptRestoresUncommittedRehydrateFailure(t *testing.T) {
	pending := interrupts.Pending{
		ParentRunID: "run_1",
		SessionID:   "ses_1",
		TurnID:      "turn_1",
		ProcessID:   "proc_1",
	}
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{
		pending: map[string]interrupts.Pending{"run_1": pending},
	}}
	turns := stubTurns{
		resumeErr:    turn.ErrTurnNotFound,
		rehydrateErr: errors.New("snapshot store temporarily unavailable"),
	}

	_, err := newCoordinator(stores, turns).ResumeClaimedInterrupt(context.Background(), "run_1", interrupts.Resolution{Approved: true}, nil)
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
	if got, ok := stores.interrupts.pending["run_1"]; !ok || got.ProcessID != "proc_1" {
		t.Fatalf("pending = %+v, ok=%v; uncommitted claim must be restored", got, ok)
	}
}

func TestResumeClaimedInterruptDoesNotRestoreCommittedRehydrateFailure(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{
		pending: map[string]interrupts.Pending{
			"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", ProcessID: "proc_1"},
		},
	}}
	turns := stubTurns{
		resumeErr:    turn.ErrTurnNotFound,
		rehydrateErr: errors.Join(turn.ErrRehydrateCommitted, errors.New("resumed process failed")),
	}

	_, err := newCoordinator(stores, turns).ResumeClaimedInterrupt(context.Background(), "run_1", interrupts.Resolution{Approved: true}, nil)
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
	if _, ok := stores.interrupts.pending["run_1"]; ok {
		t.Fatal("committed rehydrate failure must remain consumed")
	}
}
