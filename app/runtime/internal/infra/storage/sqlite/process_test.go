package sqlite_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

const storedBuildID = "test-build"

func newProcessStorage(t *testing.T) (*sql.DB, *sqlite.ProcessStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, sqlite.NewProcessStore(db)
}

func newProcessStore(t *testing.T) *sqlite.ProcessStore {
	t.Helper()
	_, store := newProcessStorage(t)
	return store
}

func validStoredSnapshot(id string, status core.ProcessStatus) core.ProcessSnapshot {
	started := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "digest"},
		StartedAt:     started,
		Status:        status,
	}
	if status == core.StatusWaiting {
		snapshot.Suspension = &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-" + id,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     started,
		}
	}
	return snapshot
}

func storedSnapshotTree(rootID string, snapshots ...core.ProcessSnapshot) execution.ProcessTreeState {
	processes := make([]execution.ProcessState, len(snapshots))
	for index, snapshot := range snapshots {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			panic(err)
		}
		processes[index] = execution.ProcessState{
			ID:        snapshot.ID,
			ParentID:  snapshot.ParentID,
			StartedAt: snapshot.StartedAt,
			Payload:   payload,
		}
	}
	return execution.ProcessTreeState{RootID: rootID, Processes: processes}
}

func decodeStoredSnapshot(t *testing.T, process execution.ProcessState) core.ProcessSnapshot {
	t.Helper()
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal(process.Payload, &snapshot); err != nil {
		t.Fatalf("decode stored process %q payload: %v", process.ID, err)
	}
	return snapshot
}

func storedUsage() accounting.Snapshot { return accounting.Snapshot{} }

func storedCheckpoint(sessionID, buildID string, usage accounting.Snapshot) execution.ProcessCheckpoint {
	return execution.ProcessCheckpoint{
		BuildID: buildID,
		Scope:   execution.TurnScope{SessionID: sessionID},
		Usage:   usage,
	}
}

func TestProcessStoreSaveLoadReplacement(t *testing.T) {
	store := newProcessStore(t)
	snapshot := validStoredSnapshot("proc-1", core.StatusWaiting)
	snapshot.Conditions = map[string]bool{"k": true}
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("conversation-1", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("first SaveTree: %v", err)
	}
	snapshot.Status = core.StatusCompleted
	snapshot.Suspension = nil
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("conversation-1", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("second SaveTree: %v", err)
	}
	tree, checkpoint, err := store.LoadTree(t.Context(), snapshot.ID)
	if err != nil || len(tree.Processes) != 1 ||
		checkpoint.BuildID != storedBuildID {
		t.Fatalf("LoadTree = %+v, checkpoint %+v, err %v", tree, checkpoint, err)
	}
	loaded := decodeStoredSnapshot(t, tree.Processes[0])
	if loaded.Status != core.StatusCompleted || !loaded.Conditions["k"] {
		t.Fatalf("loaded process snapshot = %+v", loaded)
	}
}

func TestProcessStoreRoundTripsSuspensionStateWithoutInterpretingIt(t *testing.T) {
	store := newProcessStore(t)
	snapshot := validStoredSnapshot("proc-opaque-suspension", core.StatusWaiting)
	snapshot.Suspension.FrameworkState = json.RawMessage(`{"opaque":"agent-runtime-state"}`)

	if err := store.SaveTree(
		t.Context(),
		storedSnapshotTree(snapshot.ID, snapshot),
		storedCheckpoint("conversation-1", storedBuildID, accounting.Snapshot{}),
	); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	tree, _, err := store.LoadTree(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	if len(tree.Processes) != 1 {
		t.Fatalf("loaded tree = %+v", tree)
	}
	loadedSnapshot := decodeStoredSnapshot(t, tree.Processes[0])
	if loadedSnapshot.Suspension == nil {
		t.Fatalf("loaded process snapshot = %+v", loadedSnapshot)
	}
	loaded := loadedSnapshot.Suspension
	if !bytes.Equal(loaded.FrameworkState, snapshot.Suspension.FrameworkState) {
		t.Fatalf("framework state = %s, want %s", loaded.FrameworkState, snapshot.Suspension.FrameworkState)
	}
}

func TestProcessStoreLoadMissingIsSentinel(t *testing.T) {
	store := newProcessStore(t)
	if _, _, err := store.LoadTree(t.Context(), "nope"); !errors.Is(err, execution.ErrProcessStateNotFound) {
		t.Fatalf("LoadTree(missing) err = %v", err)
	}
}

func TestProcessStoreTreatsExecutorPayloadAsOpaque(t *testing.T) {
	db, store := newProcessStorage(t)
	snapshot := validStoredSnapshot("proc-corrupt", core.StatusCompleted)
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE process_states SET payload = ? WHERE id = ?`, []byte("{"), snapshot.ID); err != nil {
		t.Fatalf("replace stored payload: %v", err)
	}
	tree, _, err := store.LoadTree(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatalf("LoadTree(opaque payload): %v", err)
	}
	if len(tree.Processes) != 1 || string(tree.Processes[0].Payload) != "{" {
		t.Fatalf("opaque payload = %+v, want exact bytes", tree.Processes)
	}
}

func TestProcessStorePersistsApplicationUsageWithRoot(t *testing.T) {
	db, store := newProcessStorage(t)
	snapshot := validStoredSnapshot("proc-usage", core.StatusCompleted)
	usage := accounting.Snapshot{Models: []accounting.ModelUsage{{
		Model:      "served-model",
		TokenUsage: accounting.TokenUsage{PromptTokens: 8, CompletionTokens: 3, ReasoningTokens: 1},
		CostUSD:    0.75,
		Calls:      2,
	}}}
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("", storedBuildID, usage)); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	_, checkpoint, err := store.LoadTree(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	if len(checkpoint.Usage.Models) != 1 || checkpoint.Usage.Models[0] != usage.Models[0] {
		t.Fatalf("usage = %+v, want %+v", checkpoint.Usage, usage)
	}

	for name, data := range map[string]string{
		"malformed":      `{`,
		"unknown field":  `{"models":[],"future":true}`,
		"null models":    `{"models":null}`,
		"trailing value": `{"models":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("", storedBuildID, usage)); err != nil {
				t.Fatalf("restore valid tree: %v", err)
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE process_states SET usage = ? WHERE id = ?`, data, snapshot.ID); err != nil {
				t.Fatalf("corrupt usage: %v", err)
			}
			if _, _, err := store.LoadTree(t.Context(), snapshot.ID); !errors.Is(err, execution.ErrInvalidProcessTreeState) {
				t.Fatalf("LoadTree(corrupt usage) err = %v, want ErrInvalidProcessTreeState", err)
			}
		})
	}
}

func TestProcessStorePersistsApplicationTurnScopeWithRoot(t *testing.T) {
	db, store := newProcessStorage(t)
	snapshot := validStoredSnapshot("proc-scope", core.StatusCompleted)
	want := execution.TurnScope{
		SessionID:   "session-scope",
		Cwd:         "/workspace/scope",
		Isolated:    true,
		GoalLeaseID: "goal-lease-scope",
	}
	checkpoint := storedCheckpoint(want.SessionID, storedBuildID, accounting.Snapshot{})
	checkpoint.Scope = want
	checkpoint.Provider = "openai"
	checkpoint.Budget = accounting.Budget{MaxTokens: 1234, MaxCostUSD: 2.5, MaxSteps: 7}
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), checkpoint); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	_, got, err := store.LoadTree(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	if got.Scope != want || got.Provider != checkpoint.Provider || got.Budget != checkpoint.Budget {
		t.Fatalf("checkpoint = %+v, want scope=%+v provider=%q budget=%+v", got, want, checkpoint.Provider, checkpoint.Budget)
	}

	for name, data := range map[string]string{
		"malformed":         `{`,
		"unknown field":     `{"scope":{"session_id":"","cwd":"","isolated":false,"goal_lease_id":"","future":true},"provider":"","budget":{"max_tokens":0,"max_cost_usd":0,"max_steps":0}}`,
		"unstable identity": `{"scope":{"session_id":" session","cwd":"","isolated":false,"goal_lease_id":""},"provider":"","budget":{"max_tokens":0,"max_cost_usd":0,"max_steps":0}}`,
		"trailing value":    `{"scope":{"session_id":"","cwd":"","isolated":false,"goal_lease_id":""},"provider":"","budget":{"max_tokens":0,"max_cost_usd":0,"max_steps":0}} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), checkpoint); err != nil {
				t.Fatalf("restore valid tree: %v", err)
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE process_states SET policy = ? WHERE id = ?`, data, snapshot.ID); err != nil {
				t.Fatalf("corrupt policy: %v", err)
			}
			if _, _, err := store.LoadTree(t.Context(), snapshot.ID); !errors.Is(err, execution.ErrInvalidProcessTreeState) {
				t.Fatalf("LoadTree(corrupt policy) err = %v, want ErrInvalidProcessTreeState", err)
			}
		})
	}
}

func TestProcessStoreRejectsMixedBuildIdentity(t *testing.T) {
	db, store := newProcessStorage(t)
	root := validStoredSnapshot("mixed-build-root", core.StatusCompleted)
	child := validStoredSnapshot("mixed-build-child", core.StatusCompleted)
	child.ParentID = root.ID
	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	if _, err := db.ExecContext(
		t.Context(),
		`UPDATE process_states SET build_id = ? WHERE id = ?`,
		"other-build",
		child.ID,
	); err != nil {
		t.Fatalf("corrupt child build identity: %v", err)
	}
	if _, _, err := store.LoadTree(t.Context(), root.ID); !errors.Is(err, execution.ErrInvalidProcessTreeState) {
		t.Fatalf("LoadTree(mixed build) error = %v, want ErrInvalidProcessTreeState", err)
	}
}

func TestProcessStoreRejectsMissingBuildIdentityBeforeMutation(t *testing.T) {
	store := newProcessStore(t)
	snapshot := validStoredSnapshot("missing-build", core.StatusCompleted)
	if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("", "", accounting.Snapshot{})); err == nil {
		t.Fatal("SaveTree accepted an empty build identity")
	}
	if _, _, err := store.LoadTree(t.Context(), snapshot.ID); !errors.Is(err, execution.ErrProcessStateNotFound) {
		t.Fatalf("LoadTree after rejected save = %v, want ErrProcessStateNotFound", err)
	}
}

func TestProcessStoreDeleteIgnoresUnrelatedCorruptSnapshot(t *testing.T) {
	db, store := newProcessStorage(t)
	corrupt := validStoredSnapshot("corrupt", core.StatusCompleted)
	target := validStoredSnapshot("target", core.StatusCompleted)
	for _, snapshot := range []core.ProcessSnapshot{corrupt, target} {
		if err := store.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE process_states SET payload = ? WHERE id = ?`, []byte("{"), corrupt.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTrees(t.Context(), []string{target.ID}); err != nil {
		t.Fatalf("DeleteTrees: %v", err)
	}
	if _, _, err := store.LoadTree(t.Context(), target.ID); !errors.Is(err, execution.ErrProcessStateNotFound) {
		t.Fatalf("LoadTree deleted target: %v", err)
	}
	corruptTree, _, err := store.LoadTree(t.Context(), corrupt.ID)
	if err != nil || len(corruptTree.Processes) != 1 || string(corruptTree.Processes[0].Payload) != "{" {
		t.Fatalf("unrelated opaque payload changed: tree=%+v err=%v", corruptTree, err)
	}
}

func TestProcessStoreReplaceRemovesStaleDescendants(t *testing.T) {
	store := newProcessStore(t)
	root := validStoredSnapshot("root", core.StatusCompleted)
	child := validStoredSnapshot("child", core.StatusKilled)
	child.ParentID = root.ID
	grandchild := validStoredSnapshot("grandchild", core.StatusKilled)
	grandchild.ParentID = child.ID
	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child, grandchild), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatal(err)
	}
	tree, _, err := store.LoadTree(t.Context(), root.ID)
	if err != nil || len(tree.Processes) != 1 || tree.Processes[0].ID != root.ID {
		t.Fatalf("tree after replacement = %+v, %v", tree, err)
	}
	for _, stale := range []string{child.ID, grandchild.ID} {
		if _, _, err := store.LoadTree(t.Context(), stale); !errors.Is(err, execution.ErrProcessStateNotFound) {
			t.Fatalf("stale descendant %q survived replacement: %v", stale, err)
		}
	}
}

func TestProcessStoreDoesNotCreateProductLineageWithoutConversation(t *testing.T) {
	db, store := newProcessStorage(t)
	root := validStoredSnapshot("root-unattached", core.StatusWaiting)
	child := validStoredSnapshot("child-unattached", core.StatusWaiting)
	child.ParentID = root.ID

	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	if _, err := sqlite.NewSessionStore(db).Get(t.Context(), child.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("unattached child session lookup = %v, want ErrNotFound", err)
	}
}

func TestProcessStoreMissingConversationRollsBackTreeAndLineage(t *testing.T) {
	db, store := newProcessStorage(t)
	root := validStoredSnapshot("root-missing-conversation", core.StatusWaiting)
	child := validStoredSnapshot("child-missing-conversation", core.StatusWaiting)
	child.ParentID = root.ID

	err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child), storedCheckpoint("missing-conversation", storedBuildID, accounting.Snapshot{}))
	if !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("SaveTree error = %v, want ErrNotFound", err)
	}
	if _, _, err := store.LoadTree(t.Context(), root.ID); !errors.Is(err, execution.ErrProcessStateNotFound) {
		t.Fatalf("rolled-back process tree lookup = %v, want ErrProcessStateNotFound", err)
	}
	if _, err := sqlite.NewSessionStore(db).Get(t.Context(), child.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("rolled-back child session lookup = %v, want ErrNotFound", err)
	}
}

func TestProcessStorePersistsDelegationLineageInSameWrite(t *testing.T) {
	db, store := newProcessStorage(t)
	sessions := sqlite.NewSessionStore(db)
	parent, err := sessions.Create(t.Context(), "Parent", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	root := validStoredSnapshot("root", core.StatusWaiting)
	child := validStoredSnapshot("child", core.StatusWaiting)
	child.ParentID = root.ID
	leaf := validStoredSnapshot("leaf", core.StatusCompleted)
	leaf.ParentID = child.ID

	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child, leaf), storedCheckpoint(parent.ID, storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	storedChild, err := sessions.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedChild.Kind != session.KindSubtask || storedChild.ParentID != parent.ID || storedChild.Cwd != parent.Cwd {
		t.Fatalf("child lineage = %#v", storedChild)
	}
	storedLeaf, err := sessions.Get(t.Context(), leaf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedLeaf.Kind != session.KindSubtask || storedLeaf.ParentID != child.ID {
		t.Fatalf("leaf lineage = %#v", storedLeaf)
	}
}

func TestProcessStoreOwnsCheckpointCaptureMetadata(t *testing.T) {
	db, store := newProcessStorage(t)
	sessions := sqlite.NewSessionStore(db)
	parent, err := sessions.Create(t.Context(), "Parent", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	root := validStoredSnapshot("capture-root", core.StatusWaiting)
	child := validStoredSnapshot("capture-child", core.StatusCompleted)
	child.ParentID = root.ID
	beforeCommit := time.Now().UTC().Add(-time.Second)

	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child), storedCheckpoint(parent.ID, storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	rows, err := db.QueryContext(t.Context(),
		`SELECT payload, committed_at FROM process_states WHERE id IN (?, ?) ORDER BY id`,
		root.ID,
		child.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var committedAt int64
	for rows.Next() {
		var body []byte
		var committed int64
		if err := rows.Scan(&body, &committed); err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		if _, leaked := wire["captured_at"]; leaked {
			t.Fatal("stored Agent snapshot contains application capture metadata")
		}
		if time.Unix(0, committed).Before(beforeCommit) {
			t.Fatalf("committed_at = %s, want storage commit time", time.Unix(0, committed))
		}
		if committedAt != 0 && committedAt != committed {
			t.Fatalf("one process-tree commit has timestamps %d and %d", committedAt, committed)
		}
		committedAt = committed
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if committedAt == 0 {
		t.Fatal("process tree has no storage capture metadata")
	}
	storedChild, err := sessions.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedChild.UpdatedAt.UnixNano() != committedAt {
		t.Fatalf("subtask audit time = %s, want process-tree commit %s", storedChild.UpdatedAt, time.Unix(0, committedAt))
	}
}

func TestProcessStoreDeleteTreeRemovesDescendantsOnly(t *testing.T) {
	store := newProcessStore(t)
	root := validStoredSnapshot("root", core.StatusCompleted)
	child := validStoredSnapshot("child", core.StatusKilled)
	child.ParentID = root.ID
	unrelated := validStoredSnapshot("unrelated", core.StatusCompleted)
	if err := store.SaveTree(t.Context(), storedSnapshotTree(root.ID, root, child), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTree(t.Context(), storedSnapshotTree(unrelated.ID, unrelated), storedCheckpoint("", storedBuildID, accounting.Snapshot{})); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTrees(t.Context(), []string{root.ID}); err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []string{root.ID, child.ID} {
		if _, _, err := store.LoadTree(t.Context(), deleted); !errors.Is(err, execution.ErrProcessStateNotFound) {
			t.Fatalf("deleted process %q survived: %v", deleted, err)
		}
	}
	tree, _, err := store.LoadTree(t.Context(), unrelated.ID)
	if err != nil || len(tree.Processes) != 1 || tree.Processes[0].ID != unrelated.ID {
		t.Fatalf("unrelated tree = %+v, %v", tree, err)
	}
}
