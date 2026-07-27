package arch

import (
	"encoding/json"
	"testing"
	"time"

	agentcore "github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

func representativeAgentWireContracts(t *testing.T) map[string]any {
	t.Helper()

	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("inspect the deployment")))
	if err != nil {
		t.Fatal(err)
	}
	request.Tools = []chat.ToolDefinition{{
		Name:        "lookup",
		Description: "Look up deployment state",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}

	assistant := chat.NewAssistantMessage(
		chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{"id":"agent-1"}`}),
		chat.NewToolCallPart(chat.ToolCall{ID: "call-2", Name: "lookup", Arguments: `{"id":"agent-2"}`}),
	)
	response, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &assistant,
		FinishReason: chat.FinishReasonToolCalls,
	})
	if err != nil {
		t.Fatal(err)
	}
	response.ID = "response-1"
	response.Model = "fixture-model"

	completedResult := chat.ToolResult{ID: "call-1", Name: "lookup", Result: "deployed"}
	prompt := json.RawMessage(`{"message":"operator approval required"}`)
	resumeSchema := json.RawMessage(`{"type":"string"}`)
	checkpoint := &toolloop.Checkpoint{
		SchemaVersion:      toolloop.CheckpointSchemaVersion,
		ID:                 "approval-1",
		Round:              2,
		MaxRounds:          50,
		MaxConcurrentCalls: 8,
		ToolsetDigest:      "8f4d804e6c3d359b39e96baba43b430e1c81381ed321e8ce8e66bb32cb5e00f4",
		Request:            request,
		Response:           response,
		CallStates: []toolloop.CallCheckpoint{
			{Status: toolloop.CallCompleted, Result: &completedResult},
			{
				Status: toolloop.CallPaused,
				Pending: &toolloop.PendingCall{
					ID:           "approval-1",
					Reason:       "operator approval required",
					Prompt:       prompt,
					ResumeSchema: resumeSchema,
				},
			},
		},
		NextResult: 1,
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatal(err)
	}

	startedAt := time.Date(2026, time.July, 15, 8, 30, 0, 123_000_000, time.UTC)
	processSnapshot := agentcore.ProcessSnapshot{
		SchemaVersion: agentcore.ProcessSnapshotSchemaVersion,
		ID:            "process-1",
		ParentID:      "process-root",
		Deployment: agentcore.DeploymentRef{
			Name:    "researcher",
			Version: "0.4.0-fixture",
			Digest:  "f2389de79afc8d79fe4f8ac35e7e66e195cf4a73762c3f6a7c454ef72e84bfdf",
		},
		StartedAt: startedAt,
		Status:    agentcore.StatusWaiting,
		Suspension: &interaction.Suspension{
			SchemaVersion:  interaction.SuspensionSchemaVersion,
			ID:             checkpoint.ID,
			Kind:           interaction.SuspensionTool,
			Prompt:         prompt,
			ResumeSchema:   resumeSchema,
			FrameworkState: json.RawMessage(`{"owner":"framework-fixture"}`),
			CreatedAt:      startedAt.Add(4 * time.Minute),
		},
		GoalName: "answer-question",
		OwnUsage: agentcore.Usage{Cost: 0.0125, Tokens: 321, ModelCalls: 2, Actions: 1},
		Blackboard: map[string]agentcore.TaggedValue{
			"answer": {Type: "string", Value: json.RawMessage(`"pending"`)},
			"input":  {Type: "fixture.Input", Value: json.RawMessage(`{"query":"lynx"}`)},
		},
		Conditions: map[string]bool{"approved": false, "researched": true},
		Objects: []agentcore.TaggedValue{{
			Type:  "fixture.Result",
			Value: json.RawMessage(`{"title":"Lynx"}`),
		}},
	}

	return map[string]any{
		"process_failure":     agentcore.ProcessFailure{Message: "provider unavailable"},
		"process_snapshot":    processSnapshot,
		"toolloop_checkpoint": checkpoint,
	}
}
