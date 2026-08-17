package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func TestClaimIdleSessionHoldsAndReleasesSession(t *testing.T) {
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}}}
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
			pending: map[string]runs.Pending{
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
	stores := coordinatorStores{interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}}}
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
			pending: map[string]runs.Pending{
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
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	selection := runfixture.Selection()
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{
			"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", ExecutorID: "turn_1",
				Capabilities: run.Capabilities{
					InterruptKinds: []interrupt.Kind{interrupt.Question},
				},
				Interrupts: []transcript.Interrupt{{
					ItemID: "item_1", ItemOccurredAt: createdAt,
					RunID: "run_1", Kind: interrupt.Question, Question: question,
				}},
				Bindings: []runs.InterruptBinding{{
					InterruptItemID: "item_1", MemberID: "member_1", RequestID: "request_1",
				}},
				Continuations: []runs.Continuation{{
					RunID: "run_1", MemberID: "member_1", RunCreatedAt: createdAt, ModelSelection: selection,
				}},
				CreatedAt: createdAt.Add(time.Second),
			},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi"))},
			Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
				ID: "run_1", SessionID: "ses_1", State: run.Waiting,
				ModelSelection: selection,
				Capabilities: run.Capabilities{
					InterruptKinds: []interrupt.Kind{interrupt.Question},
				},
				CreatedAt: createdAt, MessageMark: -1,
			})},
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.QuestionItem, OccurredAt: createdAt,
				Question: question,
			})},
		},
		terminal: &applied,
	}

	terminal, err := newCoordinator(stores, nil).ApplyRunCancel(t.Context(), "ses_1", "run_1", "user stopped", finishedAt)
	if err != nil {
		t.Fatalf("ApplyRunCancel: %v", err)
	}
	if terminal.ID() != "run_1" || terminal.State() != run.Canceled {
		t.Fatalf("returned terminal run = %+v, want canceled run_1", terminal)
	}
	appliedRoot, ok := applied.RootRun()
	if !ok {
		t.Fatal("terminal plan has no root Run")
	}
	outcome, terminalized := appliedRoot.Outcome()
	if appliedRoot.State() != run.Canceled || !terminalized || outcome != run.OutcomeCanceled {
		t.Fatalf("terminal Run = %+v, want canceled", appliedRoot)
	}
	if appliedRoot.Detail() != "user stopped" || !appliedRoot.FinishedAt().Equal(finishedAt) {
		t.Fatalf("terminal detail/time = %q/%v", appliedRoot.Detail(), appliedRoot.FinishedAt())
	}
	if appliedRoot.MessageMark() != 2 {
		t.Fatalf("terminal mark = %d, want 2", appliedRoot.MessageMark())
	}
	if len(applied.Items) != 0 {
		t.Fatalf("interrupt items = %+v, want complete Question prompt left unchanged", applied.Items)
	}
	if applied.CheckpointRootID != "member_1" {
		t.Fatalf("checkpoint root = %q, want member_1 in cancel write-set", applied.CheckpointRootID)
	}
}

func TestApplyRunCancelSettlesQuestionToolAndClosesModelContext(t *testing.T) {
	finishedAt := time.Date(2026, 8, 11, 2, 3, 4, 0, time.UTC)
	createdAt := finishedAt.Add(-time.Minute)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Favorite color?"}}}
	selection := runfixture.Selection()
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{
			"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", ExecutorID: "turn_1",
				Capabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
				Interrupts: []transcript.Interrupt{{
					ItemID: "item_question", ItemOccurredAt: createdAt,
					RunID: "run_1", Kind: interrupt.Question, Question: question,
				}},
				Bindings: []runs.InterruptBinding{{
					InterruptItemID: "item_question", MemberID: "member_1", RequestID: "request_1",
				}},
				Continuations: []runs.Continuation{{
					RunID: "run_1", MemberID: "member_1", RunCreatedAt: createdAt,
					ModelSelection: selection,
					DrainedTools: []runs.DrainedTool{{
						ItemID: "item_tool", ItemOccurredAt: createdAt,
						CallID: "tool:runtime:0", Name: "ask_user", Arguments: "{}",
					}},
				}},
				CreatedAt: createdAt.Add(time.Second),
			},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{
				chat.NewUserMessage(chat.NewTextPart("ask me")),
				chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
					ID: "provider_call_1", Name: "ask_user", Arguments: "{}",
				})),
			},
			Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
				ID: "run_1", SessionID: "ses_1", State: run.Waiting,
				ModelSelection: selection,
				Capabilities:   run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
				CreatedAt:      createdAt, MessageMark: run.UnknownMessageMark,
			})},
			Items: []transcript.Item{
				itemfixture.MustRestore(itemfixture.Input{
					ID: "item_tool", RunID: "run_1", SessionID: "ses_1",
					Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
					Tool: &transcript.ToolInvocation{Name: "ask_user"},
				}),
				itemfixture.MustRestore(itemfixture.Input{
					ID: "item_question", RunID: "run_1", SessionID: "ses_1",
					Kind: transcript.QuestionItem, OccurredAt: createdAt, Question: question,
				}),
			},
		},
		terminal: &applied,
	}

	terminal, err := newCoordinator(stores, nil).ApplyRunCancel(
		t.Context(), "ses_1", "run_1", "user dismissed the question", finishedAt,
	)
	if err != nil {
		t.Fatalf("ApplyRunCancel: %v", err)
	}
	if terminal.State() != run.Canceled || terminal.MessageMark() != 3 {
		t.Fatalf("terminal Run = %+v, want canceled at message mark 3", terminal)
	}
	if len(applied.Items) != 1 || applied.Items[0].ID() != "item_tool" ||
		applied.Items[0].Status() != transcript.ItemIncomplete {
		t.Fatalf("terminal Items = %+v, want incomplete item_tool", applied.Items)
	}
	if len(applied.Messages) != 1 || applied.Messages[0].Role != chat.RoleTool ||
		len(applied.Messages[0].Parts) != 1 || applied.Messages[0].Parts[0].ToolResult == nil {
		t.Fatalf("terminal Messages = %+v, want one Tool result", applied.Messages)
	}
	result := *applied.Messages[0].Parts[0].ToolResult
	if result.ID != "provider_call_1" || result.Name != "ask_user" || !result.IsError ||
		result.Result != "tool call canceled before completion: user dismissed the question" {
		t.Fatalf("terminal Tool result = %+v", result)
	}
}

func TestApplyRunLostProjectsTerminalTranscript(t *testing.T) {
	finishedAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)
	createdAt := finishedAt.Add(-time.Minute)
	costUSD := 0.75
	approval := &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium"}
	selection := runfixture.Selection()
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{
			"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", ExecutorID: "turn_1", GoalIncarnationID: "lease_1",
				Capabilities: run.Capabilities{
					InterruptKinds: []interrupt.Kind{interrupt.Approval},
				},
				Interrupts: []transcript.Interrupt{{
					ItemID: "item_1", ItemOccurredAt: createdAt,
					RunID: "run_1", Kind: interrupt.Approval, Approval: approval,
				}},
				Bindings: []runs.InterruptBinding{{
					InterruptItemID: "item_1", MemberID: "member_1", RequestID: "request_1",
					ToolCallID: "call_1",
				}},
				Continuations: []runs.Continuation{{
					RunID: "run_1", MemberID: "member_1", RunCreatedAt: createdAt,
					ModelSelection: selection,
					Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 4, Usage: &accounting.Usage{
						Total: accounting.Totals{CostUSD: &costUSD},
					}}),
				}},
				CreatedAt: createdAt.Add(time.Second),
			},
		}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
			Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
				ID: "run_1", SessionID: "ses_1", State: run.Waiting,
				GoalIncarnationID: "lease_1",
				ModelSelection:    selection,
				Capabilities: run.Capabilities{
					InterruptKinds: []interrupt.Kind{interrupt.Approval},
				},
				Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 4, Usage: &accounting.Usage{
					Total: accounting.Totals{CostUSD: &costUSD},
				}}),
				CreatedAt: createdAt, MessageMark: -1,
			})},
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				ID: "item_1", RunID: "run_1", SessionID: "ses_1",
				Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
				Tool: &transcript.ToolInvocation{Name: "shell"},
			})},
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
	outcome, terminalized := appliedRoot.Outcome()
	if appliedRoot.State() != run.Failed || !terminalized || outcome != run.OutcomeLost {
		t.Fatalf("terminal Run = %+v, want failed/lost", appliedRoot)
	}
	// This path is the only one that knows the run died parked on an interrupt
	// with an unrestorable snapshot, so it is the one that has to say so. Leaving
	// the detail empty pushed the sentence out to the presenter, which could only
	// see the kind and wrote one default for every way a run can be lost.
	failure, failed := appliedRoot.Failure()
	if !failed || failure.Kind != run.FailureLost ||
		failure.Detail != "the parked Run tree's executor checkpoint could not be restored" {
		t.Fatalf("terminal failure = %+v, want run_lost naming its cause", failure)
	}
	if len(applied.Items) != 1 {
		t.Fatalf("terminal items = %+v, want one incomplete ToolCall", applied.Items)
	}
	toolFailure, toolFailed := applied.Items[0].Failure()
	if applied.Items[0].Status() != transcript.ItemIncomplete || !toolFailed ||
		toolFailure.Detail != "tool call abandoned because its run could not be resumed" {
		t.Fatalf("terminal items = %+v, want incomplete failed tool naming its cause", applied.Items)
	}
	if applied.CheckpointRootID != "member_1" || !appliedRoot.FinishedAt().Equal(finishedAt) || appliedRoot.MessageMark() != 1 {
		t.Fatalf("terminal plan = %+v", applied)
	}
	if applied.GoalRun == nil || applied.GoalRun.SessionID != "ses_1" ||
		applied.GoalRun.IncarnationID != "lease_1" || applied.GoalRun.RunID != "run_1" ||
		applied.GoalRun.Outcome != run.OutcomeLost || applied.GoalRun.CostUSD != costUSD ||
		applied.GoalRun.Steps != 4 || !applied.GoalRun.CompletedAt.Equal(finishedAt) {
		t.Fatalf("terminal Goal Run = %+v", applied.GoalRun)
	}
}

func TestApplyRunLostTerminalizesWholeParkedTreeInPostorder(t *testing.T) {
	createdAt := time.Date(2026, 7, 17, 2, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue child?"}}}
	childLineage := run.Lineage{
		SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root",
	}
	selection := runfixture.Selection()
	pending := runs.Pending{
		RootRunID: "run_root", SessionID: "ses_1", ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: createdAt,
			RunID: "run_child", Kind: interrupt.Question, Question: question,
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question", MemberID: "member_child", RequestID: "request_child",
		}},
		Continuations: []runs.Continuation{
			{
				RunID: "run_child", MemberID: "member_child",
				Lineage: childLineage, RunCreatedAt: createdAt, ModelSelection: selection,
			},
			{RunID: "run_root", MemberID: "member_root", RunCreatedAt: createdAt, ModelSelection: selection},
		},
		CreatedAt: createdAt.Add(time.Second),
	}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{"run_root": pending}},
		snapshot: Snapshot{
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
			Runs: []run.Run{
				runfixture.MustRestore(run.Snapshot{
					ID: "run_root", SessionID: "ses_1", State: run.Waiting,
					ModelSelection: selection,
					Capabilities:   pending.Capabilities,
					CreatedAt:      createdAt, MessageMark: run.UnknownMessageMark,
				}),
				runfixture.MustRestore(run.Snapshot{
					ID: "run_child", SessionID: "ses_1", State: run.Waiting,
					ModelSelection: selection,
					Capabilities:   pending.Capabilities,
					Lineage:        childLineage,
					CreatedAt:      createdAt, MessageMark: run.UnknownMessageMark,
				}),
			},
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				ID: "item_question", SessionID: "ses_1", RunID: "run_child",
				Kind:     transcript.QuestionItem,
				Question: question, OccurredAt: createdAt,
			})},
		},
		terminal: &applied,
	}
	corruptSnapshot := stores.snapshot
	corruptSnapshot.Runs = append([]run.Run(nil), stores.snapshot.Runs...)
	corrupt := corruptSnapshot.Runs[1].Snapshot()
	corrupt.Capabilities = run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	corruptSnapshot.Runs[1] = runfixture.MustRestore(corrupt)
	corruptApplied := TerminalPlan{}
	corruptStores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{"run_root": pending}},
		snapshot:   corruptSnapshot,
		terminal:   &corruptApplied,
	}
	if err := newCoordinator(corruptStores, nil).ApplyRunLost(t.Context(), "ses_1", "run_root", finishedAt); err == nil {
		t.Fatal("ApplyRunLost accepted a child Run run capabilities that differs from root admission")
	}
	if len(corruptApplied.Runs) != 0 {
		t.Fatalf("child policy drift reached terminal commit: %+v", corruptApplied)
	}

	if err := newCoordinator(stores, nil).ApplyRunLost(t.Context(), "ses_1", "run_root", finishedAt); err != nil {
		t.Fatalf("ApplyRunLost: %v", err)
	}
	if len(applied.Runs) != 2 || applied.Runs[0].ID() != "run_child" || applied.Runs[1].ID() != "run_root" {
		t.Fatalf("terminal Run order = %+v, want child then root", applied.Runs)
	}
	for _, record := range applied.Runs {
		outcome, terminalized := record.Outcome()
		failure, failed := record.Failure()
		if record.State() != run.Failed || !terminalized || outcome != run.OutcomeLost ||
			!failed || failure.Kind != run.FailureLost || !record.FinishedAt().Equal(finishedAt) {
			t.Fatalf("terminal Run = %+v", record)
		}
	}
	if len(applied.Items) != 0 {
		t.Fatalf("terminal Items = %+v, want complete Question prompt left unchanged", applied.Items)
	}
}

// TestApplyRunLostRejectsContinuationFactDriftBeforeTerminalCommit proves
// parked_continuation_matches_run_facts at online parked-tree termination: a
// corrupt hand-off cannot be consumed or converted into a terminal history.
func TestApplyRunLostRejectsContinuationFactDriftBeforeTerminalCommit(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	pendingInterrupt := transcript.Interrupt{
		ItemID: "item_question", ItemOccurredAt: createdAt, RunID: "run_root",
		Kind: interrupt.Question, Question: question,
	}
	capabilities := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	pending := runs.Pending{
		RootRunID: "run_root", SessionID: "ses_1", ExecutorID: "turn_1",
		Capabilities: capabilities,
		Interrupts:   []transcript.Interrupt{pendingInterrupt},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: pendingInterrupt.ItemID,
			MemberID:        "member_root",
			RequestID:       "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_root", MemberID: "member_root", RunCreatedAt: createdAt,
			ModelSelection: runfixture.Selection(), Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2}),
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	var applied TerminalPlan
	stores := coordinatorStores{
		interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{"run_root": pending}},
		snapshot: Snapshot{
			Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
				ID: "run_root", SessionID: "ses_1", State: run.Waiting,
				Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Steps: 1}), Capabilities: capabilities,
				CreatedAt: createdAt, MessageMark: run.UnknownMessageMark,
			})},
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				ID: pendingInterrupt.ItemID, SessionID: "ses_1", RunID: "run_root",
				Kind:     transcript.QuestionItem,
				Question: question, OccurredAt: createdAt,
			})},
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
