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
		MaxConcurrentCalls: toolloop.DefaultMaxConcurrentCalls,
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

	pause := &toolloop.Pause{ID: checkpoint.ID, Reason: "operator approval required", Prompt: prompt, ResumeSchema: resumeSchema, Checkpoint: checkpoint}
	resume := &toolloop.Resume{ID: checkpoint.ID, Input: json.RawMessage(`"approved"`)}
	events := []toolloop.Event{
		{Kind: toolloop.EventModelRequest, Round: 2, Request: request},
		{Kind: toolloop.EventModelResponse, Round: 2, Response: response},
		{Kind: toolloop.EventToolCall, Round: 2, ToolCall: &chat.ToolCall{ID: "call-2", Name: "lookup", Arguments: `{"id":"agent-2"}`}},
		{Kind: toolloop.EventToolResult, Round: 2, ToolResult: &completedResult},
		{Kind: toolloop.EventPause, Round: 2, Pause: pause},
		{Kind: toolloop.EventResume, Round: 2, Resume: resume},
	}
	for i := range events {
		if err := events[i].Validate(); err != nil {
			t.Fatalf("events[%d]: %v", i, err)
		}
	}

	startedAt := time.Date(2026, time.July, 15, 8, 30, 0, 123_000_000, time.UTC)
	capturedAt := startedAt.Add(5 * time.Minute)
	processSnapshot := agentcore.ProcessSnapshot{
		SchemaVersion: agentcore.ProcessSnapshotSchemaVersion,
		Revision:      7,
		ID:            "process-1",
		ParentID:      "process-root",
		Depth:         1,
		Deployment: agentcore.DeploymentRef{
			Name:    "researcher",
			Version: "0.4.0-fixture",
			Digest:  "f2389de79afc8d79fe4f8ac35e7e66e195cf4a73762c3f6a7c454ef72e84bfdf",
		},
		StartedAt:  startedAt,
		CapturedAt: capturedAt,
		Status:     agentcore.StatusWaiting,
		Suspension: &interaction.Suspension{
			SchemaVersion: interaction.SuspensionSchemaVersion,
			ID:            checkpoint.ID,
			Kind:          interaction.SuspensionTool,
			Prompt:        prompt,
			ResumeSchema:  resumeSchema,
			Payload:       json.RawMessage(`{"owner":"interaction-fixture"}`),
			CreatedAt:     startedAt.Add(4 * time.Minute),
		},
		GoalName: "answer-question",
		History: []agentcore.ActionRunSnapshot{{
			ActionName: "lookup",
			StartedAt:  startedAt.Add(time.Minute),
			Duration:   250 * time.Millisecond,
			Status:     agentcore.ActionSucceeded,
			Attempts:   2,
		}},
		OwnCost:   0.0125,
		OwnTokens: 321,
		OwnModelCalls: []agentcore.ModelCall{{
			Timestamp:        startedAt.Add(2 * time.Minute),
			Model:            "fixture-model",
			Provider:         "fixture-provider",
			CostUSD:          0.01,
			PromptTokens:     200,
			CompletionTokens: 100,
			ReasoningTokens:  20,
			Duration:         2 * time.Second,
			ActionName:       "lookup",
		}},
		OwnEmbeddingCalls: []agentcore.EmbeddingCall{{
			Timestamp:   startedAt.Add(3 * time.Minute),
			Model:       "fixture-embedding",
			Provider:    "fixture-provider",
			CostUSD:     0.0025,
			InputTokens: 21,
			InputCount:  2,
			Duration:    50 * time.Millisecond,
			ActionName:  "lookup",
		}},
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

	metadata, err := agentcore.ParseSessionMetadata([]byte(`{
		"channel":"fixture",
		"locale":"zh-CN",
		"nested":{"durable":true}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	session := agentcore.Session{
		ID:        "session-1",
		ParentID:  "session-root",
		UserID:    "user-1",
		AgentName: "researcher",
		StartedAt: startedAt,
		UpdatedAt: capturedAt,
		Metadata:  metadata,
	}
	interactionEvents := []interaction.Event{
		{Kind: interaction.EventModelRequest, Round: 2, Request: request},
		{Kind: interaction.EventModelResponse, Round: 2, Response: response},
		{Kind: interaction.EventToolCall, Round: 2, ToolCall: &chat.ToolCall{ID: "call-2", Name: "lookup", Arguments: `{"id":"agent-2"}`}},
		{Kind: interaction.EventToolResult, Round: 2, ToolResult: &completedResult},
		{Kind: interaction.EventPause, Round: 2, Suspension: processSnapshot.Suspension.Clone()},
		{Kind: interaction.EventResume, Round: 2, Resume: &interaction.Resume{ID: checkpoint.ID, Input: json.RawMessage(`"approved"`)}},
	}
	for i := range interactionEvents {
		if err := interactionEvents[i].Validate(); err != nil {
			t.Fatalf("interactionEvents[%d]: %v", i, err)
		}
	}

	return map[string]any{
		"interaction_events":  interactionEvents,
		"process_snapshot":    processSnapshot,
		"session":             session,
		"toolloop_checkpoint": checkpoint,
		"toolloop_events":     events,
	}
}
