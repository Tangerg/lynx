package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestClaimRunSlotHoldsAndReleasesSession(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	admission, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimRunSlot(context.Background(), "ses_1")
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

	_, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimRunSlot(context.Background(), "ses_1")
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

	_, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimRunSlot(context.Background(), "ses_1")
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

	admission, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimMutationSlot("ses_1")
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
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{
			"run_1": {RunID: "run_1", SessionID: "ses_1", ProcessID: "proc_1"},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi"))},
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
		terminal: &applied,
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

func TestApplyRunLostProjectsTerminalTranscript(t *testing.T) {
	finishedAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{
			"run_1": {RunID: "run_1", SessionID: "ses_1", ProcessID: "proc_1"},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
			Runs: []transcript.Run{{
				ID: "run_1", SessionID: "ses_1", State: execution.Interrupted,
				Interrupts:  []transcript.Interrupt{{ItemID: "item_1", Kind: transcript.ApprovalInterrupt}},
				MessageMark: -1,
			}},
			Items: []transcript.Item{{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.ToolCall, Status: transcript.ItemRunning,
			}},
		},
		terminal: &applied,
	}

	err := newCoordinator(stores, nil).ApplyRunLost(t.Context(), "ses_1", "run_1", finishedAt)
	if err != nil {
		t.Fatalf("ApplyRunLost: %v", err)
	}
	if applied.Run.State != execution.Failed || applied.Run.Outcome == nil || *applied.Run.Outcome != execution.OutcomeError {
		t.Fatalf("terminal run = %+v, want failed/error", applied.Run)
	}
	if applied.Run.Result == nil || applied.Run.Result.Error == nil || applied.Run.Result.Error.Kind != transcript.RunLostProblem || applied.Run.Result.Error.Detail != "" {
		t.Fatalf("terminal result = %+v, want run_lost", applied.Run.Result)
	}
	if len(applied.Items) != 1 || applied.Items[0].Status != transcript.ItemIncomplete || applied.Items[0].Error == nil || applied.Items[0].Error.Detail != "" {
		t.Fatalf("terminal items = %+v, want incomplete failed tool", applied.Items)
	}
	if applied.ProcessID != "proc_1" || !applied.Run.FinishedAt.Equal(finishedAt) || applied.Run.MessageMark != 1 {
		t.Fatalf("terminal plan = %+v", applied)
	}
}
