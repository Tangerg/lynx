package runtimeembedded

import (
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

func TestProjectEventSkipsEphemeralFramesAndClassifiesStreams(t *testing.T) {
	if _, include, err := projectEvent(protocol.RunEvent{Event: protocol.StreamEvent{
		Type: protocol.StreamSegmentProgress, Progress: &protocol.RunProgress{Activity: "thinking"},
	}}); err != nil || include {
		t.Fatalf("progress = (include %v, error %v)", include, err)
	}

	streamError := errors.New("broken stream")
	stream := projectEventStream(func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{}, streamError)
	})
	for _, err := range stream {
		if !errors.Is(err, streamError) {
			t.Fatalf("stream error = %v", err)
		}
		return
	}
	t.Fatal("stream yielded no error")
}

func TestRunProfileRejectsShapeChangingFeatures(t *testing.T) {
	err := validateRunProfile(protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
	})
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("error = %v, want ErrIncompatibleRuntime", err)
	}
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
