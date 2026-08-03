package sessions

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestSnapshotNormalizeForRestoreProjectsPreviewWithoutMutatingSource(t *testing.T) {
	snapshot := offloadedSnapshot("full body")

	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		t.Fatalf("NormalizeForRestore: %v", err)
	}
	if got, _ := normalized.Items[0].Tool.Result.String(); got != "bounded preview" {
		t.Fatalf("normalized result = %q, want bounded preview", got)
	}
	if got, _ := snapshot.Items[0].Tool.Result.String(); got != "full body" {
		t.Fatalf("source result mutated to %q", got)
	}
	if normalized.Items[0].Tool == snapshot.Items[0].Tool {
		t.Fatal("normalization reused the source tool invocation pointer")
	}
}

func TestSnapshotValidateToolResultsRejectsBrokenRelationships(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{
			name: "missing blob",
			mutate: func(snapshot *Snapshot) {
				snapshot.ToolResults = nil
			},
			want: "references missing tool result",
		},
		{
			name: "detached blob",
			mutate: func(snapshot *Snapshot) {
				snapshot.Items[0].Tool.Offload = nil
			},
			want: "references missing transcript item",
		},
		{
			name: "foreign session",
			mutate: func(snapshot *Snapshot) {
				snapshot.ToolResults[0].SessionID = "ses_other"
			},
			want: "belongs to session",
		},
		{
			name: "unrelated result",
			mutate: func(snapshot *Snapshot) {
				result := tool.StringResult("neither preview nor body")
				snapshot.Items[0].Tool.Result = &result
			},
			want: "matches neither",
		},
		{
			name: "duplicate item binding",
			mutate: func(snapshot *Snapshot) {
				duplicate := snapshot.ToolResults[0]
				duplicate.ID = "OTHER234"
				snapshot.ToolResults = append(snapshot.ToolResults, duplicate)
			},
			want: "multiple tool results",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := offloadedSnapshot("full body")
			tt.mutate(&snapshot)
			if err := snapshot.ValidateToolResults(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateToolResults() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func offloadedSnapshot(result string) Snapshot {
	ref := &offload.Ref{ID: "BLOB234"}
	value := tool.StringResult(result)
	return Snapshot{
		Session: session.Session{ID: "ses_1"},
		Items: []transcript.Item{{
			SessionID: "ses_1", ID: "item_1", Kind: transcript.ToolCall,
			Tool: &transcript.ToolInvocation{Name: "shell", Result: &value, Offload: ref},
		}},
		ToolResults: []offload.ToolResultBlob{{
			ID: "BLOB234", SessionID: "ses_1", ItemID: "item_1", ToolName: "shell",
			Preview: "bounded preview", Body: "full body", CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
}

// The archive's run lineage carries rules a schema cannot state: JSON Schema
// cannot compare two fields, and "root" is the ABSENCE of the child edges, which
// no presence rule can condition on. So they are checked where the archive
// becomes a session — before anything is written.
func TestPortableSnapshotRefusesABrokenRunLineage(t *testing.T) {
	profile := execution.RunProtocolProfile{}
	root := func() PortableRun {
		return PortableRun{
			SessionID: "ses_1", ID: "run_root", Outcome: execution.OutcomeCompleted,
			ProtocolProfile: &profile,
		}
	}
	for name, runs := range map[string][]PortableRun{
		// A root with no profile is an archive that lost the contract its run
		// published under. Defaulting it to empty would import a different run.
		"root without a protocol profile": {
			{SessionID: "ses_1", ID: "run_root", Outcome: execution.OutcomeCompleted},
		},
		// A child reads its root's contract; one of its own is a second statement of
		// something the archive already says once.
		"child with its own protocol profile": {root(), {
			SessionID: "ses_1", ID: "run_child", Outcome: execution.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_root",
			ProtocolProfile: &profile,
		}},
		"child naming itself as its own root": {root(), {
			SessionID: "ses_1", ID: "run_child", Outcome: execution.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_child",
		}},
		// A child whose root is not in the archive imports a tree that cannot be
		// walked — and a contract that cannot be read.
		"child whose root is absent": {{
			SessionID: "ses_1", ID: "run_child", Outcome: execution.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_gone", RootRunID: "run_gone",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			portable := PortableSnapshot{
				Session: PortableSession{ID: "ses_1", Title: "t", Cwd: "/w"},
				Runs:    runs,
			}
			if _, err := portable.CanonicalSnapshot(); !errors.Is(err, ErrInvalidPortableSnapshot) {
				t.Fatalf("CanonicalSnapshot err = %v, want ErrInvalidPortableSnapshot", err)
			}
		})
	}
}

// A child inherits its root's contract rather than carrying one, so the restored
// run must come out holding the ROOT's profile — not an empty one.
func TestPortableSnapshotChildInheritsItsRootsProfile(t *testing.T) {
	profile := execution.RunProtocolProfile{
		ChildRuns:      true,
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	}
	at := time.Unix(1, 0).UTC()
	portable := PortableSnapshot{
		Session: PortableSession{ID: "ses_1", Title: "t", Cwd: "/w", CreatedAt: at, UpdatedAt: at},
		// The spawning item has to exist: a child run is spawned BY something, and an
		// archive naming an item it does not contain is a tree that cannot be walked.
		// The spawning item is a TOOL CALL: a child run is the execution of one.
		Items: []transcript.Item{{
			SessionID: "ses_1", RunID: "run_root", ID: "item_1", CreatedAt: at,
			Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
			Tool: &transcript.ToolInvocation{Name: "delegate_task"},
		}},
		Runs: []PortableRun{
			{
				SessionID: "ses_1", ID: "run_root", Outcome: execution.OutcomeCompleted,
				ProtocolProfile: &profile,
				CreatedAt:       at, FinishedAt: at, UpdatedAt: at,
			},
			{
				SessionID: "ses_1", ID: "run_child", Outcome: execution.OutcomeCompleted,
				SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_root",
				CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			},
		},
	}
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		t.Fatalf("CanonicalSnapshot: %v", err)
	}
	for _, run := range snapshot.Runs {
		if !run.ProtocolProfile.ChildRuns || len(run.ProtocolProfile.InterruptKinds) != 1 {
			t.Fatalf("run %q profile = %+v, want the root's", run.ID, run.ProtocolProfile)
		}
	}
}
