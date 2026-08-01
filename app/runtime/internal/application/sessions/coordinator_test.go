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

func TestClaimIdleSessionHoldsAndReleasesSession(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	admission, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimIdleSession(context.Background(), "ses_1")
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

func TestClaimIdleSessionRejectsOpenInterrupt(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {RootRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	_, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimIdleSession(context.Background(), "ses_1")
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

func TestClaimIdleSessionRejectsActiveClaim(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{claimed: map[string]bool{"ses_1": true}}

	_, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimIdleSession(context.Background(), "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("err = %v, want ErrSessionBusy", err)
	}
	if len(claimer.released) != 0 {
		t.Fatalf("released = %v, want none", claimer.released)
	}
}

func TestClaimSessionMutationAllowsOpenInterrupt(t *testing.T) {
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {RootRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	admission, err := newCoordinatorWithAdmissions(stores, nil, claimer).ClaimSessionMutation("ses_1")
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
	createdAt := finishedAt.Add(-time.Minute)
	question := &transcript.Question{Prompt: "Continue?"}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{
			"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
				ProtocolProfile: execution.RunProtocolProfile{
					InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
				},
				Interrupts: []transcript.Interrupt{{
					ItemID: "item_1", RunID: "run_1", Kind: execution.QuestionInterrupt, Question: question,
				}},
				Suspensions: []interrupts.SuspensionBinding{{
					InterruptItemID: "item_1", ProcessID: "proc_1", SuspensionID: "suspension_1",
				}},
				Continuations: []interrupts.Continuation{{
					RunID: "run_1", ProcessID: "proc_1", RunCreatedAt: createdAt,
				}},
				CreatedAt: createdAt.Add(time.Second),
			},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi"))},
			Runs: []transcript.Run{{
				ID: "run_1", SessionID: "ses_1", State: execution.Interrupted,
				ProtocolProfile: execution.RunProtocolProfile{
					InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
				},
				Interrupts: []transcript.Interrupt{{ItemID: "item_1", Kind: execution.QuestionInterrupt}},
				CreatedAt:  createdAt, MessageMark: -1,
			}},
			Items: []transcript.Item{{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.QuestionItem, Status: transcript.ItemRunning, CreatedAt: createdAt,
				Question: question,
			}},
		},
		terminal: &applied,
	}

	terminal, err := newCoordinator(stores, nil).ApplyRunCancel(t.Context(), "ses_1", "run_1", "user stopped", finishedAt)
	if err != nil {
		t.Fatalf("ApplyRunCancel: %v", err)
	}
	if terminal.ID != "run_1" || terminal.State != execution.Canceled {
		t.Fatalf("returned terminal run = %+v, want canceled run_1", terminal)
	}
	appliedRoot, ok := applied.RootRun()
	if !ok {
		t.Fatal("terminal plan has no root Run")
	}
	if appliedRoot.State != execution.Canceled || appliedRoot.Outcome == nil || *appliedRoot.Outcome != execution.OutcomeCanceled {
		t.Fatalf("terminal Run = %+v, want canceled", appliedRoot)
	}
	if appliedRoot.Detail != "user stopped" || !appliedRoot.FinishedAt.Equal(finishedAt) {
		t.Fatalf("terminal detail/time = %q/%v", appliedRoot.Detail, appliedRoot.FinishedAt)
	}
	if appliedRoot.MessageMark != 2 || len(appliedRoot.Interrupts) != 0 {
		t.Fatalf("terminal mark/interrupts = %d/%+v, want 2/none", appliedRoot.MessageMark, appliedRoot.Interrupts)
	}
	if len(applied.Items) != 1 || applied.Items[0].Status != transcript.ItemIncomplete {
		t.Fatalf("interrupt items = %+v, want one incomplete item", applied.Items)
	}
	if applied.CheckpointRootID != "proc_1" {
		t.Fatalf("checkpoint root = %q, want proc_1 in cancel write-set", applied.CheckpointRootID)
	}
}

func TestApplyRunLostProjectsTerminalTranscript(t *testing.T) {
	finishedAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)
	createdAt := finishedAt.Add(-time.Minute)
	costUSD := 0.75
	approval := &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{
			"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", GoalLeaseID: "lease_1",
				ProtocolProfile: execution.RunProtocolProfile{
					InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
				},
				Interrupts: []transcript.Interrupt{{
					ItemID: "item_1", RunID: "run_1", Kind: execution.ApprovalInterrupt, Approval: approval,
				}},
				Suspensions: []interrupts.SuspensionBinding{{
					InterruptItemID: "item_1", ProcessID: "proc_1", SuspensionID: "suspension_1",
				}},
				Continuations: []interrupts.Continuation{{
					RunID: "run_1", ProcessID: "proc_1", RunCreatedAt: createdAt,
					Metrics: transcript.RunMetrics{Steps: 4, Usage: &transcript.Usage{
						ModelUsage: transcript.ModelUsage{CostUSD: &costUSD},
					}},
				}},
				CreatedAt: createdAt.Add(time.Second),
			},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
			Runs: []transcript.Run{{
				ID: "run_1", SessionID: "ses_1", State: execution.Interrupted,
				GoalLeaseID: "lease_1",
				ProtocolProfile: execution.RunProtocolProfile{
					InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
				},
				Metrics: transcript.RunMetrics{Steps: 4, Usage: &transcript.Usage{
					ModelUsage: transcript.ModelUsage{CostUSD: &costUSD},
				}},
				Interrupts: []transcript.Interrupt{{ItemID: "item_1", Kind: execution.ApprovalInterrupt}},
				CreatedAt:  createdAt, MessageMark: -1,
			}},
			Items: []transcript.Item{{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.ToolCall, Status: transcript.ItemRunning, CreatedAt: createdAt,
				Tool: &transcript.ToolInvocation{Name: "shell"},
			}},
		},
		terminal: &applied,
	}

	err := newCoordinator(stores, nil).ApplyRunLost(t.Context(), "ses_1", "run_1", finishedAt)
	if err != nil {
		t.Fatalf("ApplyRunLost: %v", err)
	}
	appliedRoot, ok := applied.RootRun()
	if !ok {
		t.Fatal("terminal plan has no root Run")
	}
	if appliedRoot.State != execution.Failed || appliedRoot.Outcome == nil || *appliedRoot.Outcome != execution.OutcomeError {
		t.Fatalf("terminal Run = %+v, want failed/error", appliedRoot)
	}
	// This path is the only one that knows the run died parked on an interrupt
	// with an unrestorable snapshot, so it is the one that has to say so. Leaving
	// the detail empty pushed the sentence out to the presenter, which could only
	// see the kind and wrote one default for every way a run can be lost.
	if appliedRoot.Error == nil ||
		appliedRoot.Error.Kind != transcript.RunLostProblem ||
		appliedRoot.Error.Detail != "the parked Run tree's executor checkpoint could not be restored" {
		t.Fatalf("terminal failure = %+v, want run_lost naming its cause", appliedRoot.Error)
	}
	if len(applied.Items) != 1 || applied.Items[0].Status != transcript.ItemIncomplete ||
		applied.Items[0].Error == nil ||
		applied.Items[0].Error.Detail != "tool call abandoned because its run could not be resumed" {
		t.Fatalf("terminal items = %+v, want incomplete failed tool naming its cause", applied.Items)
	}
	if applied.CheckpointRootID != "proc_1" || !appliedRoot.FinishedAt.Equal(finishedAt) || appliedRoot.MessageMark != 1 {
		t.Fatalf("terminal plan = %+v", applied)
	}
	if applied.GoalTurn == nil || applied.GoalTurn.SessionID != "ses_1" ||
		applied.GoalTurn.LeaseID != "lease_1" || applied.GoalTurn.RunID != "run_1" ||
		applied.GoalTurn.Outcome != execution.OutcomeError || applied.GoalTurn.CostUSD != costUSD ||
		applied.GoalTurn.Steps != 4 || !applied.GoalTurn.CompletedAt.Equal(finishedAt) {
		t.Fatalf("terminal Goal turn = %+v", applied.GoalTurn)
	}
}

func TestApplyRunLostTerminalizesWholeParkedTreeInPostorder(t *testing.T) {
	createdAt := time.Date(2026, 7, 17, 2, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	question := &transcript.Question{Prompt: "Continue child?"}
	childLineage := execution.RunLineage{
		SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root",
	}
	pending := interrupts.Pending{
		RootRunID: "run_root", SessionID: "ses_1", TurnID: "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			ChildRuns: true, InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", RunID: "run_child", Kind: execution.QuestionInterrupt, Question: question,
		}},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: "item_question", ProcessID: "proc_child", SuspensionID: "suspension_child",
		}},
		Continuations: []interrupts.Continuation{
			{
				RunID: "run_child", ProcessID: "proc_child",
				Lineage: childLineage, RunCreatedAt: createdAt,
			},
			{RunID: "run_root", ProcessID: "proc_root", RunCreatedAt: createdAt},
		},
		CreatedAt: createdAt.Add(time.Second),
	}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{"run_root": pending}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
			Runs: []transcript.Run{
				{
					ID: "run_root", SessionID: "ses_1", State: execution.Interrupted,
					ProtocolProfile: pending.ProtocolProfile,
					CreatedAt:       createdAt, MessageMark: transcript.UnknownMessageMark,
				},
				{
					ID: "run_child", SessionID: "ses_1", State: execution.Interrupted,
					ProtocolProfile: pending.ProtocolProfile,
					SpawnedByItemID: childLineage.SpawnedByItemID,
					ParentRunID:     childLineage.ParentRunID, RootRunID: childLineage.RootRunID,
					CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
				},
			},
			Items: []transcript.Item{{
				ID: "item_question", SessionID: "ses_1", RunID: "run_child",
				Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
				Question: question, CreatedAt: createdAt,
			}},
		},
		terminal: &applied,
	}
	corruptSnapshot := stores.snapshot
	corruptSnapshot.Runs = append([]transcript.Run(nil), stores.snapshot.Runs...)
	corruptSnapshot.Runs[1].ProtocolProfile = execution.RunProtocolProfile{
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	}
	corruptApplied := TerminalPlan{}
	corruptStores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{"run_root": pending}},
		snapshot:   corruptSnapshot,
		terminal:   &corruptApplied,
	}
	if err := newCoordinator(corruptStores, nil).ApplyRunLost(t.Context(), "ses_1", "run_root", finishedAt); err == nil {
		t.Fatal("ApplyRunLost accepted a child Run protocol profile that differs from root admission")
	}
	if len(corruptApplied.Runs) != 0 {
		t.Fatalf("child policy drift reached terminal commit: %+v", corruptApplied)
	}

	if err := newCoordinator(stores, nil).ApplyRunLost(t.Context(), "ses_1", "run_root", finishedAt); err != nil {
		t.Fatalf("ApplyRunLost: %v", err)
	}
	if len(applied.Runs) != 2 || applied.Runs[0].ID != "run_child" || applied.Runs[1].ID != "run_root" {
		t.Fatalf("terminal Run order = %+v, want child then root", applied.Runs)
	}
	for _, run := range applied.Runs {
		if run.State != execution.Failed || run.Outcome == nil || *run.Outcome != execution.OutcomeError ||
			run.Error == nil || run.Error.Kind != transcript.RunLostProblem || !run.FinishedAt.Equal(finishedAt) {
			t.Fatalf("terminal Run = %+v", run)
		}
	}
	if len(applied.Items) != 1 || applied.Items[0].RunID != "run_child" ||
		applied.Items[0].Status != transcript.ItemIncomplete {
		t.Fatalf("terminal Items = %+v", applied.Items)
	}
}

// TestApplyRunLostRejectsContinuationFactDriftBeforeTerminalCommit proves
// parked_continuation_matches_run_facts at online parked-tree termination: a
// corrupt hand-off cannot be consumed or converted into a terminal history.
func TestApplyRunLostRejectsContinuationFactDriftBeforeTerminalCommit(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	question := &transcript.Question{Prompt: "Continue?"}
	interrupt := transcript.Interrupt{
		ItemID: "item_question", RunID: "run_root",
		Kind: execution.QuestionInterrupt, Question: question,
	}
	profile := execution.RunProtocolProfile{
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	}
	pending := interrupts.Pending{
		RootRunID: "run_root", SessionID: "ses_1", TurnID: "turn_1",
		ProtocolProfile: profile,
		Interrupts:      []transcript.Interrupt{interrupt},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: interrupt.ItemID,
			ProcessID:       "process_root",
			SuspensionID:    "suspension_root",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: "run_root", ProcessID: "process_root", RunCreatedAt: createdAt,
			Metrics: transcript.RunMetrics{Steps: 2},
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]interrupts.Pending{"run_root": pending}},
		snapshot: Snapshot{
			Runs: []transcript.Run{{
				ID: "run_root", SessionID: "ses_1", State: execution.Interrupted,
				Metrics: transcript.RunMetrics{Steps: 1}, ProtocolProfile: profile,
				CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
			}},
			Items: []transcript.Item{{
				ID: interrupt.ItemID, SessionID: "ses_1", RunID: "run_root",
				Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
				Question: question, CreatedAt: createdAt,
			}},
		},
		terminal: &applied,
	}

	err := newCoordinator(stores, nil).ApplyRunLost(
		t.Context(), "ses_1", "run_root", createdAt.Add(time.Minute),
	)
	if err == nil {
		t.Fatal("ApplyRunLost accepted cumulative metrics that differ from the Run")
	}
	if len(applied.Runs) != 0 {
		t.Fatalf("contradictory continuation reached terminal commit: %+v", applied)
	}
	if _, found := stores.interrupts.pending[pending.RootRunID]; !found {
		t.Fatal("failed validation consumed the open Pending set")
	}
}
