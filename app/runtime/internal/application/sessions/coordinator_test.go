package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

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
				"run_1": {RunID: "run_1", SessionID: "ses_1"},
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
				"run_1": {RunID: "run_1", SessionID: "ses_1"},
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

func TestApplyRunCancelProjectsTerminalTranscript(t *testing.T) {
	finishedAt := time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)
	var applied CancelPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{
			"run_1": {RunID: "run_1", SessionID: "ses_1", ProcessID: "proc_1"},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage("hello"), chat.NewAssistantMessage("hi")},
			Runs: []transcript.Run{{
				ID: "run_1", SessionID: "ses_1", State: execution.Interrupted,
				Interrupts:  []transcript.Interrupt{{ItemID: "item_1", Kind: transcript.QuestionInterrupt}},
				MessageMark: -1,
			}},
			Items: []transcript.Item{{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
			}},
		},
		canceled: &applied,
	}

	err := newCoordinator(stores, nil).ApplyRunCancel(t.Context(), "ses_1", "run_1", "user stopped", finishedAt)
	if err != nil {
		t.Fatalf("ApplyRunCancel: %v", err)
	}
	if applied.Run.State != execution.Canceled || applied.Run.Outcome == nil || *applied.Run.Outcome != execution.OutcomeCanceled {
		t.Fatalf("terminal run = %+v, want canceled", applied.Run)
	}
	if applied.Run.Result == nil || applied.Run.Detail != "user stopped" || !applied.Run.FinishedAt.Equal(finishedAt) {
		t.Fatalf("terminal result/detail/time = %+v/%q/%v", applied.Run.Result, applied.Run.Detail, applied.Run.FinishedAt)
	}
	if applied.Run.MessageMark != 2 || len(applied.Run.Interrupts) != 0 {
		t.Fatalf("terminal mark/interrupts = %d/%+v, want 2/none", applied.Run.MessageMark, applied.Run.Interrupts)
	}
	if len(applied.Items) != 1 || applied.Items[0].Status != transcript.ItemIncomplete {
		t.Fatalf("interrupt items = %+v, want one incomplete item", applied.Items)
	}
	if applied.ProcessID != "proc_1" {
		t.Fatalf("process snapshot = %q, want proc_1 in cancel write-set", applied.ProcessID)
	}
}
