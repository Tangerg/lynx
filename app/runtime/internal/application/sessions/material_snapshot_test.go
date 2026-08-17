package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

func TestMaterialSnapshotAcceptsOneCoherentMountedReadModel(t *testing.T) {
	if err := validMaterialSnapshot().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want coherent snapshot", err)
	}
}

func TestMaterialSnapshotRejectsOpenInterruptWithoutTranscriptItem(t *testing.T) {
	snapshot := validMaterialSnapshot()
	snapshot.Items = nil

	err := snapshot.Validate()
	if err == nil || !strings.Contains(err.Error(), "interrupt Item \"item_question\"") {
		t.Fatalf("Validate() error = %v, want missing interrupt Item", err)
	}
}

func TestMaterialSnapshotRejectsContradictoryPendingProjection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MaterialSnapshot)
		want   string
	}{
		{
			name: "interrupt occurrence differs from Item identity",
			mutate: func(snapshot *MaterialSnapshot) {
				snapshot.Interrupts[0].Interrupts[0].ItemOccurredAt = snapshot.Interrupts[0].CreatedAt
			},
			want: "is not the exact Item",
		},
		{
			name: "question payload differs from Item",
			mutate: func(snapshot *MaterialSnapshot) {
				current := snapshot.Items[0]
				snapshot.Items[0] = itemfixture.MustRestore(itemfixture.Input{
					ID: current.ID(), SessionID: current.SessionID(), RunID: current.RunID(),
					Kind: transcript.QuestionItem, OccurredAt: current.OccurredAt(),
					Question: &transcript.Question{
						Fields: []transcript.QuestionField{{Prompt: "Different?"}},
					},
				})
			},
			want: "malformed question Item",
		},
		{
			name: "continuation differs from Run",
			mutate: func(snapshot *MaterialSnapshot) {
				snapshot.Interrupts[0].Continuations[0].RunCreatedAt = snapshot.Interrupts[0].CreatedAt
			},
			want: "continuation creation times differ",
		},
		{
			name: "waiting Run has no Pending owner",
			mutate: func(snapshot *MaterialSnapshot) {
				snapshot.Interrupts = nil
			},
			want: "has no Pending owner",
		},
		{
			name: "waiting Run has an unclaimed running Item",
			mutate: func(snapshot *MaterialSnapshot) {
				snapshot.Items = append(snapshot.Items, itemfixture.MustRestore(itemfixture.Input{
					ID: "item_orphan_tool", SessionID: "ses_1", RunID: "run_root",
					Kind: transcript.ToolCall, Status: transcript.ItemRunning,
					OccurredAt: snapshot.Interrupts[0].CreatedAt,
					Tool:       &transcript.ToolInvocation{Name: "orphan"},
				}))
			},
			want: "has no matching interrupt or drained Tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := validMaterialSnapshot()
			tt.mutate(&snapshot)
			err := snapshot.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestMaterialSnapshotRejectsResolvedApprovalThatStillClaimsPendingOwnership(t *testing.T) {
	snapshot := validApprovalMaterialSnapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("coherent approval Validate() error = %v", err)
	}
	invocation, _ := snapshot.Items[0].ToolInvocation()
	snapshot.Items[0] = itemfixture.MustRestore(itemfixture.Input{
		ID: "item_approval", SessionID: "ses_1", RunID: "run_root",
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: snapshot.Items[0].OccurredAt(), Tool: &invocation,
		ApprovalDecision: approval.Allow,
	})

	err := snapshot.Validate()
	if err == nil || !strings.Contains(err.Error(), "malformed approval Item") {
		t.Fatalf("Validate() error = %v, want partially resolved approval rejected", err)
	}
}

func TestMaterialSnapshotRejectsRunningItemOwnedByTerminalRun(t *testing.T) {
	snapshot := validApprovalMaterialSnapshot()
	createdAt := snapshot.Runs[0].CreatedAt()
	finishedAt := createdAt.Add(time.Minute)
	outcome := run.OutcomeCompleted
	snapshot.Runs[0] = runfixture.MustRestore(run.Snapshot{
		ID: "run_root", SessionID: "ses_1", State: run.Completed,
		ModelSelection: runfixture.Selection(), Capabilities: snapshot.Runs[0].Capabilities(),
		Outcome: &outcome, CreatedAt: createdAt, FinishedAt: finishedAt,
		UpdatedAt: finishedAt, MessageMark: 0,
	})
	snapshot.Interrupts = nil

	err := snapshot.Validate()
	if err == nil || !strings.Contains(err.Error(), "terminal Run Item \"item_approval\" is still running") {
		t.Fatalf("Validate() error = %v, want terminal Run/running Item contradiction rejected", err)
	}
}

func TestMaterialSnapshotRejectsGoalFromAnotherSession(t *testing.T) {
	snapshot := validMaterialSnapshot()
	snapshot.Goal = &goal.Goal{
		SessionID: "ses_other", Objective: "wrong owner", Status: goal.StatusActive,
		IncarnationID: "goal_other", Revision: 1,
		CreatedAt: snapshot.Runs[0].CreatedAt(), UpdatedAt: snapshot.Runs[0].CreatedAt(),
	}

	err := snapshot.Validate()
	if err == nil || !strings.Contains(err.Error(), "Goal belongs to Session \"ses_other\"") {
		t.Fatalf("Validate() error = %v, want foreign Goal rejected", err)
	}
}

func validMaterialSnapshot() MaterialSnapshot {
	createdAt := time.Date(2026, 8, 17, 1, 2, 3, 0, time.UTC)
	selection := runfixture.Selection()
	capabilities := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	question := &transcript.Question{
		Fields: []transcript.QuestionField{{Prompt: "Continue?"}},
	}
	return MaterialSnapshot{
		Session: sessionfixture.MustRestore(session.Snapshot{ID: "ses_1", CWD: "/workspace"}),
		Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
			ID: "run_root", SessionID: "ses_1", State: run.Waiting,
			ModelSelection: selection, Capabilities: capabilities,
			CreatedAt: createdAt, MessageMark: run.UnknownMessageMark,
		})},
		Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
			ID: "item_question", SessionID: "ses_1", RunID: "run_root",
			Kind: transcript.QuestionItem, Question: question, OccurredAt: createdAt,
		})},
		Interrupts: []runs.Pending{{
			RootRunID: "run_root", SessionID: "ses_1", ExecutorID: "executor_root",
			Capabilities: capabilities,
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_question", ItemOccurredAt: createdAt,
				RunID: "run_root", Kind: interrupt.Question, Question: question,
			}},
			Bindings: []runs.InterruptBinding{{
				InterruptItemID: "item_question", MemberID: "member_root", RequestID: "request_question",
			}},
			Continuations: []runs.Continuation{{
				RunID: "run_root", MemberID: "member_root",
				ModelSelection: selection, RunCreatedAt: createdAt,
			}},
			CreatedAt: createdAt.Add(time.Second),
		}},
	}
}

func validApprovalMaterialSnapshot() MaterialSnapshot {
	snapshot := validMaterialSnapshot()
	capabilities := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	invocation := transcript.ToolInvocation{Name: "shell"}
	pendingApproval := &transcript.Approval{
		Tool: invocation, Risk: "medium", Reason: "writes files", Rememberable: true,
	}
	snapshot.Runs[0] = runfixture.MustRestore(run.Snapshot{
		ID: "run_root", SessionID: "ses_1", State: run.Waiting,
		ModelSelection: runfixture.Selection(), Capabilities: capabilities,
		CreatedAt: snapshot.Runs[0].CreatedAt(), MessageMark: run.UnknownMessageMark,
	})
	snapshot.Items[0] = itemfixture.MustRestore(itemfixture.Input{
		ID: "item_approval", SessionID: "ses_1", RunID: "run_root",
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: snapshot.Items[0].OccurredAt(), Tool: &invocation,
	})
	snapshot.Interrupts[0].Capabilities = capabilities
	snapshot.Interrupts[0].Interrupts = []transcript.Interrupt{{
		ItemID: "item_approval", ItemOccurredAt: snapshot.Items[0].OccurredAt(),
		RunID: "run_root", Kind: interrupt.Approval, Approval: pendingApproval,
	}}
	snapshot.Interrupts[0].Bindings = []runs.InterruptBinding{{
		InterruptItemID: "item_approval", MemberID: "member_root",
		RequestID: "request_approval", ToolCallID: "provider_call_1",
	}}
	return snapshot
}
