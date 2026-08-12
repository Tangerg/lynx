package runtimeembedded

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestProjectToolPreservesStructuredDetails(t *testing.T) {
	duration := int64(1250)
	tool, err := projectTool(&protocol.ToolInvocation{
		Name: "shell", Arguments: map[string]any{"command": "go test ./..."},
		Result: map[string]any{"output": "ok", "exitCode": json.Number("0")},
	}, protocol.ItemStatusCompleted, &duration, nil)
	if err != nil {
		t.Fatalf("projectTool: %v", err)
	}
	if tool.Kind != agent.ToolShell || tool.Command != "go test ./..." || tool.Output != "ok" ||
		tool.ExitCode == nil || *tool.ExitCode != 0 || tool.Duration != 1250*time.Millisecond {
		t.Fatalf("tool = %+v", tool)
	}
}

func TestProjectToolRecognizesRootRunCancellation(t *testing.T) {
	tool, err := projectTool(
		&protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{"command": "sleep 30"}},
		protocol.ItemStatusIncomplete,
		nil,
		&protocol.ProblemData{Type: protocol.ProblemToolCanceled, Detail: "run canceled"},
	)
	if err != nil {
		t.Fatalf("projectTool: %v", err)
	}
	if tool.Status != agent.ToolCanceled || tool.Output != "run canceled" {
		t.Fatalf("tool = %+v", tool)
	}
}

func TestQuestionItemAndInterruptShareProjection(t *testing.T) {
	question := &protocol.Question{Fields: []protocol.QuestionField{{
		Prompt: "Choose a strategy", Header: "Strategy", Type: protocol.QuestionFieldChoice,
		Options: []protocol.QuestionOption{{Label: "safe"}, {Label: "fast"}},
	}}}
	block, err := projectItem(protocol.Item{
		ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusCompleted,
		Type: protocol.ItemTypeQuestion, Question: question,
	})
	if err != nil {
		t.Fatalf("projectItem: %v", err)
	}
	interaction, err := projectInteraction(protocol.Interrupt{
		ItemID: "item_1", RunID: "run_1", Type: protocol.InterruptQuestion,
		Payload: &protocol.InterruptPayload{Question: question},
	})
	if err != nil {
		t.Fatalf("projectInteraction: %v", err)
	}
	if block.Question == nil || !reflect.DeepEqual(*block.Question, interaction.(agent.Question)) {
		t.Fatalf("block question = %+v, interrupt = %+v", block.Question, interaction)
	}
}

func TestProjectEventPreservesEphemeralFramesAndClassifiesStreams(t *testing.T) {
	step, contextTokens, cost := 3, int64(8_192), 0.25
	progressEvent, include, err := projectEvent(protocol.RunEvent{EventID: "progress", Event: protocol.StreamEvent{
		Type: protocol.StreamSegmentProgress,
		Progress: &protocol.RunProgress{
			Step: &step, ContextTokens: &contextTokens, Activity: "thinking",
			Usage: &protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: 12, CostUSD: &cost}},
		},
	}})
	if err != nil || !include {
		t.Fatalf("progress = (include %v, error %v)", include, err)
	}
	progress, ok := progressEvent.Event.(agent.RunProgress)
	if !ok || progress.Step == nil || *progress.Step != step || progress.ContextTokens == nil ||
		*progress.ContextTokens != contextTokens || progress.Usage == nil || progress.Usage.InputTokens != 12 ||
		progress.Usage.CostUSD == nil || *progress.Usage.CostUSD != cost || progress.Activity != "thinking" {
		t.Fatalf("projected progress = %#v", progressEvent.Event)
	}

	arguments, include, err := projectEvent(protocol.RunEvent{EventID: "arguments", Event: protocol.StreamEvent{
		Type: protocol.StreamItemDelta, ItemID: "tool_1",
		Delta: &protocol.ItemDelta{Type: protocol.DeltaToolArguments, ArgumentsTextDelta: `{"path":"/tmp`},
	}})
	if err != nil || !include || arguments.Event != (agent.ToolArgumentsDelta{BlockID: "tool_1", Text: `{"path":"/tmp`}) {
		t.Fatalf("tool arguments = %#v, include %v, error %v", arguments.Event, include, err)
	}

	customEvent, include, err := projectEvent(protocol.RunEvent{EventID: "custom", Event: protocol.StreamEvent{
		Type: protocol.StreamCustom, Name: "vendor.trace", Payload: map[string]any{"span": "abc", "sampled": true},
	}})
	if err != nil || !include {
		t.Fatalf("custom = (include %v, error %v)", include, err)
	}
	custom, ok := customEvent.Event.(agent.CustomEvent)
	if !ok || custom.Name != "vendor.trace" || !json.Valid(custom.PayloadJSON) || !bytes.Contains(custom.PayloadJSON, []byte(`"span":"abc"`)) {
		t.Fatalf("projected custom event = %#v", customEvent.Event)
	}

	streamError := errors.New("broken stream")
	stream := projectEventStream(func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{}, streamError)
	}, "seg_1")
	for _, err := range stream {
		if !errors.Is(err, streamError) {
			t.Fatalf("stream error = %v", err)
		}
		return
	}
	t.Fatal("stream yielded no error")
}

func TestRunProfileAcceptsSubagentTrees(t *testing.T) {
	err := validateRunProfile(protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
	})
	if err != nil {
		t.Fatalf("validateRunProfile: %v", err)
	}
}

func TestProjectChildRunPreservesLineage(t *testing.T) {
	projected, err := projectRun(protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_child", SessionID: "ses_1", Status: protocol.RunStatusRunning,
			SpawnedByItemID: "item_delegate", ParentRunID: "run_root", RootRunID: "run_root",
		},
		ActiveSegmentID: "seg_child",
		ProtocolProfile: protocol.RunProtocolProfile{
			RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
			InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion},
		},
	})
	if err != nil {
		t.Fatalf("projectRun: %v", err)
	}
	want := agent.RunLineage{SpawnedByBlockID: "item_delegate", ParentRunID: "run_root", RootRunID: "run_root"}
	if projected.Lineage != want {
		t.Fatalf("lineage = %+v, want %+v", projected.Lineage, want)
	}
}

func TestProjectTreeStreamRetainsProducerAndStreamSegments(t *testing.T) {
	source := func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{
			RunID: "run_root", SegmentID: "seg_root", EventID: "evt_suspend",
			Event: protocol.StreamEvent{
				Type:    protocol.StreamSegmentFinished,
				Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentSuspended},
				Metrics: &protocol.RunMetrics{},
			},
		}, nil)
	}
	for event, err := range projectEventStream(source, "seg_root") {
		if err != nil {
			t.Fatal(err)
		}
		if event.StreamSegment() != "seg_root" || event.SegmentID != "seg_root" {
			t.Fatalf("event segments = producer %s stream %s", event.SegmentID, event.StreamSegment())
		}
		if _, ok := event.Event.(agent.RunSuspended); !ok {
			t.Fatalf("event = %T, want RunSuspended", event.Event)
		}
		return
	}
	t.Fatal("tree stream yielded no event")
}

func TestProjectSnapshotKeepsPendingApprovalIdenticalToToolItem(t *testing.T) {
	tool := &protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{
		"command": "go test ./...", "description": "Run tests",
	}}
	snapshot, err := projectSnapshot(coldRead{
		session: protocol.Session{
			ID: "ses_1", Status: protocol.SessionStatusWaiting,
			Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}},
		},
		runs: []protocol.RunRef{{
			RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusWaiting},
			ProtocolProfile: protocol.RunProtocolProfile{
				RequiredFeatures: []protocol.RunProtocolFeature{},
				InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval},
			},
		}},
		items: []protocol.Item{{
			ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusRunning,
			Type: protocol.ItemTypeToolCall, Tool: tool,
		}},
		plan: &protocol.StateSnapshot{Type: protocol.StatePlan, SessionID: "ses_1", Plan: []protocol.PlanSnapshot{}},
		interrupts: []protocol.PendingInterruptSet{{
			RootRunID: "run_1", SessionID: "ses_1",
			Interrupts: []protocol.Interrupt{{
				ItemID: "item_1", RunID: "run_1", Type: protocol.InterruptApproval,
				Payload: &protocol.InterruptPayload{Tool: tool, Risk: protocol.ApprovalRiskHigh, Rememberable: true},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("projectSnapshot: %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	approval, ok := snapshot.Interactions[0].(agent.Approval)
	if !ok || snapshot.Transcript[0].Tool == nil || !reflect.DeepEqual(*snapshot.Transcript[0].Tool, *approval.Tool) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
