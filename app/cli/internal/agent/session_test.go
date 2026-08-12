package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

func testWorkspace(path string) workspace.Workspace {
	return workspace.Workspace{Path: path, ProjectRoot: path, Availability: workspace.Available}
}

func TestSessionQueryNormalizesLocalFilterIdentity(t *testing.T) {
	t.Parallel()

	normalized, err := (SessionQuery{Search: "  release notes  ", Workspace: "  /repo/work  ", Limit: 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Search != "release notes" || normalized.Workspace != "/repo/work" || normalized.Limit != 20 {
		t.Fatalf("normalized query = %+v", normalized)
	}
	for _, query := range []SessionQuery{
		{Limit: -1},
		{Workspace: "relative/workspace"},
	} {
		if _, err := query.Normalize(); err == nil {
			t.Fatalf("Normalize accepted %+v", query)
		}
	}
}

func TestSessionSnapshotRestoresDurableProjection(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: testWorkspace("/tmp/demo"), Revision: 2},
		Transcript: []Block{
			{ID: "user_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockUser, Text: "hello"},
			{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning}},
		},
		Plan:         []PlanItem{{Title: "inspect", Status: PlanActive}},
		PlanRevision: 3,
		Runs:         []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting}},
		Interactions: []Interaction{Approval{
			RunID: "run_1", ItemID: "tool_1", Title: "edit", Rememberable: true,
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
		Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: testWorkspace("/tmp/demo")},
		Runs:    []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("waiting snapshot without interactions was accepted")
	}
}

func TestSessionUpdateRequiresIdentityAndAtLeastOneValidField(t *testing.T) {
	title, workspace := "Title", "/workspace"
	for _, test := range []struct {
		name   string
		update UpdateSession
		valid  bool
	}{
		{name: "title", update: UpdateSession{SessionID: "ses_1", Title: &title}, valid: true},
		{name: "workspace", update: UpdateSession{SessionID: "ses_1", Workspace: &workspace}, valid: true},
		{name: "empty", update: UpdateSession{SessionID: "ses_1"}},
		{name: "missing identity", update: UpdateSession{Title: &title}},
		{name: "blank workspace", update: UpdateSession{SessionID: "ses_1", Workspace: new(string)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.update.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Validate = %v, valid %t", err, test.valid)
			}
		})
	}
}

func TestSessionUpdateResultMustFulfillTheCommand(t *testing.T) {
	title, path, model, favorite := "Renamed", "/workspace/new", "model-new", true
	update := UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &path, Model: &model,
		Favorite: &favorite, ExpectedRevision: 4,
	}
	valid := Session{
		ID: "ses_1", Title: title, Status: SessionIdle, Model: model,
		Workspace: testWorkspace(path), Favorite: favorite, Revision: 5,
	}
	if err := update.ValidateResult(valid); err != nil {
		t.Fatalf("valid result: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Session)
		want   string
	}{
		{name: "identity", mutate: func(result *Session) { result.ID = "ses_2" }, want: "runtime returned session"},
		{name: "revision", mutate: func(result *Session) { result.Revision = 4 }, want: "runtime returned revision"},
		{name: "title", mutate: func(result *Session) { result.Title = "Old" }, want: "runtime returned title"},
		{name: "workspace", mutate: func(result *Session) { result.Workspace = testWorkspace("/workspace/old") }, want: "runtime returned workspace"},
		{name: "model", mutate: func(result *Session) { result.Model = "model-old" }, want: "runtime returned model"},
		{name: "favorite", mutate: func(result *Session) { result.Favorite = false }, want: "runtime returned favorite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := valid
			test.mutate(&result)
			err := update.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionSnapshotRestoresAChildOwnedInterrupt(t *testing.T) {
	root := Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting}
	child := Run{
		ID: "run_child", SessionID: "ses_1", Status: RunStatusWaiting,
		Lineage: RunLineage{SpawnedByBlockID: "delegate", ParentRunID: root.ID, RootRunID: root.ID},
	}
	approval := Approval{
		RunID: child.ID, ItemID: "approval", Title: "Read generated output",
		Tool: &ToolCall{Kind: ToolRead, Name: "read", Status: ToolRunning},
	}
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: testWorkspace("/tmp/demo")},
		Runs:    []Run{root, child},
		Transcript: []Block{
			{ID: "delegate", RunID: root.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolTask, Name: "delegate_task", Status: ToolRunning}},
			{ID: approval.ItemID, RunID: child.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool},
		},
		Interactions: []Interaction{approval},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok || active.ID != root.ID {
		t.Fatalf("ActiveRun = %+v, %v", active, ok)
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if conversation.RunID() != root.ID || conversation.Interactions()[0].(Approval).RunID != child.ID {
		t.Fatalf("restored tree = root %s interactions %+v", conversation.RunID(), conversation.Interactions())
	}
}

func TestSessionSnapshotRestoresLatestFinishedRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionIdle, Workspace: testWorkspace("/tmp/demo")},
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
	lineage := RunLineage{SpawnedByBlockID: "delegate", ParentRunID: "run_root", RootRunID: "run_root"}
	for _, test := range []struct {
		name     string
		snapshot SessionSnapshot
		want     string
	}{
		{
			name: "running run with idle session",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionIdle, Workspace: testWorkspace("/tmp/demo")},
				Runs:    []Run{{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"}},
			},
		},
		{
			name: "active run before latest run",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: testWorkspace("/tmp/demo")},
				Runs: []Run{
					{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"},
					{ID: "run_2", SessionID: "ses_1", Status: RunStatusFinished, Outcome: Outcome{Status: OutcomeCompleted}},
				},
			},
		},
		{
			name: "waiting child beneath running root",
			want: "waiting beneath running root",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: testWorkspace("/tmp/demo")},
				Runs: []Run{
					{ID: "run_root", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_root"},
					{ID: "run_child", SessionID: "ses_1", Lineage: lineage, Status: RunStatusWaiting},
				},
			},
		},
		{
			name: "running child beneath waiting root",
			want: "running beneath waiting root",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionWaiting, Workspace: testWorkspace("/tmp/demo")},
				Runs: []Run{
					{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting},
					{ID: "run_child", SessionID: "ses_1", Lineage: lineage, Status: RunStatusRunning, ActiveSegmentID: "seg_child"},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.snapshot.Validate()
			if err == nil {
				t.Fatal("inconsistent snapshot was accepted")
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("snapshot error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionSnapshotRejectsRunningItemsWithoutAnActiveRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session:    Session{ID: "ses_1", Status: SessionIdle, Workspace: testWorkspace("/tmp/demo")},
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
				Session:    Session{ID: "ses_1", Status: SessionRunning, Workspace: testWorkspace("/tmp/demo")},
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
		Session:    Session{ID: "ses_1", Status: SessionIdle, Workspace: testWorkspace("/tmp/demo")},
		Transcript: []Block{{ID: "message_1", RunID: "run_missing", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "orphaned"}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("snapshot with an orphaned item was accepted")
	}
}

func TestConversationRestoresCursorlessAttachmentHead(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: testWorkspace("/tmp/demo")},
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

func TestSessionSnapshotFindsTheLastDurableAssistantText(t *testing.T) {
	snapshot := SessionSnapshot{Transcript: []Block{
		{Kind: BlockAssistant, Text: "first"},
		{Kind: BlockReasoning, Text: "internal"},
		{Kind: BlockAssistant, Text: "  final answer  "},
	}}
	text, err := snapshot.LastAssistantText()
	if err != nil || text != "final answer" {
		t.Fatalf("LastAssistantText = (%q, %v)", text, err)
	}
}

func TestConversationMatchesColdSnapshotSemantics(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Title: "Original", Status: SessionIdle, Workspace: testWorkspace("/tmp/demo")},
		Transcript: []Block{{
			ID: "answer_1", RunID: "run_1", Status: BlockStatusCompleted,
			Kind: BlockAssistant, Text: "done",
		}},
		Runs: []Run{{
			ID: "run_1", SessionID: "ses_1", Status: RunStatusFinished,
			Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 5},
		}},
		Plan: []PlanItem{{Title: "inspect", Status: PlanDone}}, PlanRevision: 2,
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}

	snapshot.Session.Title = "Renamed elsewhere"
	if !conversation.MatchesSnapshot(snapshot) {
		t.Fatal("session metadata changed the conversation identity")
	}

	tests := []struct {
		name   string
		mutate func(*SessionSnapshot)
	}{
		{name: "transcript", mutate: func(value *SessionSnapshot) { value.Transcript[0].Text = "changed" }},
		{name: "plan", mutate: func(value *SessionSnapshot) { value.Plan[0].Status = PlanActive }},
		{name: "usage", mutate: func(value *SessionSnapshot) { value.Runs[0].Usage.InputTokens++ }},
		{name: "outcome", mutate: func(value *SessionSnapshot) { value.Runs[0].Outcome.Status = OutcomeCanceled }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := snapshot
			changed.Transcript = cloneBlocks(snapshot.Transcript)
			changed.Plan = slices.Clone(snapshot.Plan)
			changed.Runs = []Run{snapshot.Runs[0].Clone()}
			test.mutate(&changed)
			if conversation.MatchesSnapshot(changed) {
				t.Fatal("semantic change matched the live conversation")
			}
		})
	}

	invalid := snapshot
	invalid.Session.Status = "broken"
	if conversation.MatchesSnapshot(invalid) {
		t.Fatal("invalid snapshot matched the live conversation")
	}
}
