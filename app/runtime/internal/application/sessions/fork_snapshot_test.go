package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

func TestCopyForkSnapshotRemapsTheCompleteVisibleRunTree(t *testing.T) {
	at := time.Unix(10, 0).UTC()
	parent := sessionfixture.MustRestore(session.Snapshot{ID: "ses_parent", CWD: "/repo"})
	child, err := parent.Fork("ses_child", "branch", at.Add(time.Hour))
	if err != nil {
		t.Fatalf("fork child Session: %v", err)
	}
	root := runfixture.MustRestore(run.Snapshot{
		SessionID: "ses_parent", ID: "run_root", State: run.Completed,
		Capabilities: run.Capabilities{ChildRuns: true}, CreatedAt: at,
		FinishedAt: at.Add(time.Second), UpdatedAt: at.Add(time.Second), MessageMark: 1,
	})
	childRun := runfixture.MustRestore(run.Snapshot{
		SessionID: "ses_parent", ID: "run_child", State: run.Completed,
		Lineage: run.Lineage{
			SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root",
		},
		Capabilities: root.Capabilities(), CreatedAt: at.Add(time.Millisecond),
		FinishedAt: at.Add(time.Second), UpdatedAt: at.Add(time.Second), MessageMark: 1,
	})
	preview := tool.StringResult("delegated preview")
	duration := time.Second
	spawningItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "ses_parent", RunID: "run_root", ID: "item_spawn",
		Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
		OccurredAt: at, FinishedAt: at.Add(duration), ExecutionDuration: &duration,
		Tool: &transcript.ToolInvocation{
			Name: "delegate_task", Result: &preview,
			Offload: &toolresult.Ref{ID: "BLOB234"},
		},
	})
	source := Snapshot{
		Session: parent,
		Messages: []chat.Message{
			chat.NewAssistantMessage(chat.NewTextPart("delegated preview")),
		},
		Runs:  []run.Run{root, childRun},
		Items: []transcript.Item{spawningItem},
		ToolResults: []toolresult.Blob{{
			ID: "BLOB234", SessionID: "ses_parent", ItemID: "item_spawn",
			ToolName: "delegate_task", Preview: "delegated preview",
			Body: "delegated full body", CreatedAt: at,
		}},
	}
	runIDs := []string{"run_copy_root", "run_copy_child"}
	coordinator := &Coordinator{
		newRunID: func() string {
			id := runIDs[0]
			runIDs = runIDs[1:]
			return id
		},
		newItemID:       func() string { return "item_copy_spawn" },
		newToolResultID: func() toolresult.ID { return "CLONE234" },
	}

	copied, err := coordinator.copyForkSnapshot(source, child, ForkBoundary{
		Messages: source.Messages,
		RunIDs:   []string{"run_root", "run_child"},
		RunID:    "run_child",
	}, nil)
	if err != nil {
		t.Fatalf("copy fork snapshot: %v", err)
	}
	if err := copied.Validate(); err != nil {
		t.Fatalf("copied snapshot is invalid: %v", err)
	}
	if len(copied.Runs) != 2 || copied.Runs[0].ID() != "run_copy_root" || copied.Runs[1].ID() != "run_copy_child" {
		t.Fatalf("copied Runs = %+v, want fresh root and child identities", copied.Runs)
	}
	lineage := copied.Runs[1].Lineage()
	if lineage.ParentRunID != "run_copy_root" || lineage.RootRunID != "run_copy_root" || lineage.SpawnedByItemID != "item_copy_spawn" {
		t.Fatalf("copied child lineage = %+v, want remapped edges", lineage)
	}
	if len(copied.Items) != 1 || copied.Items[0].SessionID() != "ses_child" || copied.Items[0].RunID() != "run_copy_root" || copied.Items[0].ID() != "item_copy_spawn" {
		t.Fatalf("copied Items = %+v, want child-owned remapped Item", copied.Items)
	}
	invocation, present := copied.Items[0].ToolInvocation()
	if !present || invocation.Offload == nil || invocation.Offload.ID != "CLONE234" {
		t.Fatalf("copied tool invocation = %+v, want remapped offload", invocation)
	}
	if len(copied.ToolResults) != 1 || copied.ToolResults[0].ID != "CLONE234" ||
		copied.ToolResults[0].SessionID != "ses_child" || copied.ToolResults[0].ItemID != "item_copy_spawn" {
		t.Fatalf("copied ToolResults = %+v, want child-owned remapped blob", copied.ToolResults)
	}
	if source.Runs[0].ID() != "run_root" || source.Items[0].ID() != "item_spawn" || source.ToolResults[0].ID != "BLOB234" {
		t.Fatal("copy mutated the source aggregate")
	}
}
