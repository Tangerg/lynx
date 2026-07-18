package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func TestSnapshotNormalizeForRestoreProjectsPreviewWithoutMutatingSource(t *testing.T) {
	snapshot := offloadedSnapshot("full body")

	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		t.Fatalf("NormalizeForRestore: %v", err)
	}
	if got := normalized.Items[0].Tool.Result; got != "bounded preview" {
		t.Fatalf("normalized result = %q, want bounded preview", got)
	}
	if got := snapshot.Items[0].Tool.Result; got != "full body" {
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
				snapshot.Items[0].Tool.Result = "neither preview nor body"
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
	return Snapshot{
		Session: session.Session{ID: "ses_1"},
		Items: []transcript.Item{{
			SessionID: "ses_1", ID: "item_1", Kind: transcript.ToolCall,
			Tool: &transcript.ToolInvocation{Name: "shell", Result: result, Offload: ref},
		}},
		ToolResults: []offload.ToolResultBlob{{
			ID: "BLOB234", SessionID: "ses_1", ItemID: "item_1", ToolName: "shell",
			Preview: "bounded preview", Body: "full body", CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
}
