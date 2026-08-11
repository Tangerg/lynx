package agent

import "testing"

func TestSessionSnapshotRestoresDurableProjection(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: "/tmp/demo", Revision: 2},
		Transcript: []Block{
			{ID: "user_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockUser, Text: "hello"},
			{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning}},
		},
		Plan:         []PlanItem{{Title: "inspect", Status: PlanActive}},
		PlanRevision: 3,
		Runs:         []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting}},
		Interactions: []Interaction{Approval{
			ItemID: "tool_1", Title: "edit", Rememberable: true,
			Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning},
		}},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if conversation.Phase() != ConversationWaiting || len(conversation.Blocks()) != 2 || len(conversation.Interactions()) != 1 {
		t.Fatalf("restored conversation = phase %v, blocks %d, interactions %d", conversation.Phase(), len(conversation.Blocks()), len(conversation.Interactions()))
	}
}

func TestSessionSnapshotRejectsWaitingWithoutInteractions(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: "/tmp/demo"},
		Runs:    []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("waiting snapshot without interactions was accepted")
	}
}

func TestSessionSnapshotRestoresLatestFinishedRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionIdle, Workspace: "/tmp/demo"},
		Runs: []Run{{
			ID: "run_1", SessionID: "ses_1", Status: RunStatusFinished,
			Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 12, OutputTokens: 3},
		}},
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if conversation.Phase() != ConversationIdle || conversation.RunID() != "run_1" || conversation.Outcome().Status != OutcomeCompleted || conversation.Usage().InputTokens != 12 {
		t.Fatalf("restored finished conversation = phase %v, run %q, outcome %+v, usage %+v", conversation.Phase(), conversation.RunID(), conversation.Outcome(), conversation.Usage())
	}
}

func TestSessionSnapshotRejectsLifecycleDrift(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot SessionSnapshot
	}{
		{
			name: "running run with idle session",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionIdle, Workspace: "/tmp/demo"},
				Runs:    []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"}},
			},
		},
		{
			name: "active run before latest run",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: "/tmp/demo"},
				Runs: []Run{
					{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"},
					{ID: "run_2", SessionID: "ses_1", Status: RunStatusFinished, Outcome: Outcome{Status: OutcomeCompleted}},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.snapshot.Validate(); err == nil {
				t.Fatal("inconsistent snapshot was accepted")
			}
		})
	}
}

func TestSessionSnapshotRejectsRunningItemsWithoutAnActiveRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session:    Session{ID: "ses_1", Status: SessionIdle, Workspace: "/tmp/demo"},
		Transcript: []Block{{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning}}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("idle snapshot with a running item was accepted")
	}
}

func TestSessionSnapshotRejectsTransientRunningItems(t *testing.T) {
	for _, kind := range []BlockKind{BlockAssistant, BlockReasoning} {
		t.Run(string(kind), func(t *testing.T) {
			snapshot := SessionSnapshot{
				Session:    Session{ID: "ses_1", Status: SessionRunning, Workspace: "/tmp/demo"},
				Transcript: []Block{{ID: "preview_1", RunID: "run_1", Status: BlockStatusRunning, Kind: kind}},
				Runs:       []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"}},
			}
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("snapshot accepted a durable running %s preview", kind)
			}
		})
	}
}

func TestSessionSnapshotRejectsItemWithoutItsRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session:    Session{ID: "ses_1", Status: SessionIdle, Workspace: "/tmp/demo"},
		Transcript: []Block{{ID: "message_1", RunID: "run_missing", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "orphaned"}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("snapshot with an orphaned item was accepted")
	}
}

func TestConversationRestoresCursorlessAttachmentHead(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: "/tmp/demo"},
		Runs:    []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"}},
	}
	stream := SegmentStream{
		RunID: "run_1", SegmentID: "seg_1", HeadEventID: "opaque-head",
		Events: func(func(RunEvent, error) bool) {},
	}
	conversation := NewConversation()
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err != nil {
		t.Fatal(err)
	}
	if conversation.Checkpoint() != "opaque-head" || conversation.Phase() != ConversationRunning {
		t.Fatalf("restored checkpoint %q, phase %v", conversation.Checkpoint(), conversation.Phase())
	}

	stream.SegmentID = "seg_other"
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err == nil {
		t.Fatal("mismatched attached stream was accepted")
	}
}
