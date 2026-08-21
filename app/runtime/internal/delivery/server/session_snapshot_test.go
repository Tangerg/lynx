package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestGetSessionSnapshotProjectsOneLiveMaterialRead(t *testing.T) {
	s, rt := rollbackHarness(t)
	s.features.plan = true
	s.features.goals = true
	putTestSession(t, rt)

	createdAt := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	if err := rt.runs.Admit(t.Context(), run.Draft{
		SegmentID: "seg_waiting", RunID: "run_waiting", SessionID: "ses_1",
		Capabilities: capabilities, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit waiting Run: %v", err)
	}
	if err := rt.runs.Suspend(t.Context(), runfixture.MustRestore(run.Snapshot{
		ID: "run_waiting", SessionID: "ses_1", State: run.Waiting,
		Capabilities: capabilities, CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark,
	})); err != nil {
		t.Fatalf("suspend waiting Run: %v", err)
	}
	question := transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	if err := rt.hist.AppendItem(t.Context(), itemfixture.MustRestore(itemfixture.Input{
		ID: "item_question", RunID: "run_waiting", SessionID: "ses_1",
		Kind: transcript.QuestionItem, OccurredAt: createdAt, Question: &question,
	})); err != nil {
		t.Fatalf("append question Item: %v", err)
	}
	if err := rt.hist.AppendItem(t.Context(), itemfixture.MustRestore(itemfixture.Input{
		ID: "item_approved_tool", RunID: "run_waiting", SessionID: "ses_1",
		Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
		OccurredAt: createdAt.Add(time.Second), FinishedAt: createdAt.Add(2 * time.Second),
		SafetyClass: tool.SafetyClassExec, ApprovalDecision: approval.Allow,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})); err != nil {
		t.Fatalf("append approved ToolCall: %v", err)
	}
	if err := rt.interrupts.Open(t.Context(), serverPending(
		"run_waiting", "ses_1", "exec_waiting", "member_waiting",
		[]transcript.Interrupt{{
			ItemID: "item_question", Kind: interrupt.Question, Question: &question,
		}},
		createdAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	state, err := (plan.State{}).Replace([]plan.Step{{
		Description: "Answer the question", Status: plan.StatusInProgress,
	}}, createdAt)
	if err != nil {
		t.Fatalf("prepare Plan: %v", err)
	}
	if err := rt.plan.Save(t.Context(), "ses_1", 0, state); err != nil {
		t.Fatalf("save Plan: %v", err)
	}
	standingGoal, err := goal.New(
		"ses_1", "Finish the recovery", modelref.Selection{}, goal.Budget{}, capabilities,
		"goal_snapshot", createdAt,
	)
	if err != nil {
		t.Fatalf("prepare Goal: %v", err)
	}
	if _, applied, err := rt.goals.Save(t.Context(), standingGoal, goal.Version{}); err != nil || !applied {
		t.Fatalf("save Goal: applied=%t err=%v", applied, err)
	}

	ctx := withClientCapabilities(protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
	})
	snapshot, err := s.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{
		SessionID: "ses_1",
	})
	if err != nil {
		t.Fatalf("GetSessionSnapshot: %v", err)
	}
	if len(snapshot.Items) != 2 || snapshot.Items[0].ID != "item_question" ||
		snapshot.Items[1].ID != "item_approved_tool" ||
		snapshot.Items[1].ApprovalDecision != protocol.ApprovalApprove {
		t.Fatalf("Items = %+v, want question and durable approved ToolCall", snapshot.Items)
	}
	if len(snapshot.Runs) != 1 || snapshot.Runs[0].ID != "run_waiting" || snapshot.Runs[0].Status != protocol.RunStatusWaiting {
		t.Fatalf("Runs = %+v, want the waiting Run", snapshot.Runs)
	}
	if len(snapshot.Interrupts) != 1 || snapshot.Interrupts[0].RootRunID != "run_waiting" {
		t.Fatalf("Interrupts = %+v, want the open waiting set", snapshot.Interrupts)
	}
	if snapshot.Plan == nil || snapshot.Plan.Revision != 1 || len(snapshot.Plan.Steps) != 1 {
		t.Fatalf("Plan = %+v, want revision 1", snapshot.Plan)
	}
	if snapshot.Goal == nil || snapshot.Goal.Objective != "Finish the recovery" || snapshot.Goal.Status != protocol.GoalActive {
		t.Fatalf("Goal = %+v, want the active standing objective", snapshot.Goal)
	}
}

func TestGetSessionSnapshotKeepsCapabilityAndExistenceRefusals(t *testing.T) {
	s, rt := rollbackHarness(t)
	putTestSession(t, rt)
	createdAt := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	if err := rt.runs.Admit(t.Context(), run.Draft{
		SegmentID: "seg_waiting", RunID: "run_waiting", SessionID: "ses_1",
		Capabilities: capabilities, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit waiting Run: %v", err)
	}
	if err := rt.runs.Suspend(t.Context(), runfixture.MustRestore(run.Snapshot{
		ID: "run_waiting", SessionID: "ses_1", State: run.Waiting,
		Capabilities: capabilities, CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark,
	})); err != nil {
		t.Fatalf("suspend waiting Run: %v", err)
	}
	if err := rt.hist.AppendItem(t.Context(), itemfixture.MustRestore(itemfixture.Input{
		ID: "interrupt_run_waiting", RunID: "run_waiting", SessionID: "ses_1",
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{
			Fields: []transcript.QuestionField{{Prompt: "Continue?"}},
		},
	})); err != nil {
		t.Fatalf("append question Item: %v", err)
	}
	if err := rt.interrupts.Open(t.Context(), serverPending(
		"run_waiting", "ses_1", "exec_waiting", "member_waiting", nil, createdAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}

	_, err := s.GetSessionSnapshot(t.Context(), protocol.GetSessionSnapshotRequest{SessionID: "ses_1"})
	assertInterruptCapabilityGap(t, err)

	_, err = s.GetSessionSnapshot(t.Context(), protocol.GetSessionSnapshotRequest{SessionID: "ses_missing"})
	if !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Fatalf("missing Session error = %v, want session_not_found", err)
	}
}

func TestGetSessionSnapshotRejectsOwnerlessInterruptMaterial(t *testing.T) {
	s, rt := rollbackHarness(t)
	putTestSession(t, rt)
	createdAt := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	if err := rt.runs.Admit(t.Context(), run.Draft{
		SegmentID: "seg_waiting", RunID: "run_waiting", SessionID: "ses_1",
		Capabilities: capabilities, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit waiting Run: %v", err)
	}
	if err := rt.runs.Suspend(t.Context(), runfixture.MustRestore(run.Snapshot{
		ID: "run_waiting", SessionID: "ses_1", State: run.Waiting,
		Capabilities: capabilities, CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark,
	})); err != nil {
		t.Fatalf("suspend waiting Run: %v", err)
	}
	if err := rt.interrupts.Open(t.Context(), serverPending(
		"run_waiting", "ses_1", "exec_waiting", "member_waiting", nil, createdAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}

	ctx := withClientCapabilities(protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
	})
	_, err := s.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: "ses_1"})
	if err == nil || !strings.Contains(err.Error(), "interrupt Item \"interrupt_run_waiting\"") {
		t.Fatalf("snapshot error = %v, want ownerless interrupt material rejected", err)
	}
}

func assertInterruptCapabilityGap(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("snapshot capability error = %v, want capability_not_negotiated", err)
	}
}
