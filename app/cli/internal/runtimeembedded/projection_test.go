package runtimeembedded

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

func TestProjectToolPreservesStructuredDetails(t *testing.T) {
	duration := int64(1250)
	started := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	tool, err := projectTool(toolProjection{invocation: &protocol.ToolInvocation{
		Name: "shell", Arguments: map[string]any{"command": "go test ./..."},
		Result: map[string]any{"output": "ok", "exitCode": json.Number("0")},
	}, status: protocol.ItemStatusCompleted, safety: protocol.SafetyClassExec,
		startedAt: started, finishedAt: finished, durationMillis: &duration})
	if err != nil {
		t.Fatalf("projectTool: %v", err)
	}
	if tool.Kind != agent.ToolShell || tool.Command != "go test ./..." || tool.Output != "ok" ||
		tool.Safety != agent.ToolSafetyExec || !tool.StartedAt.Equal(started) || !tool.FinishedAt.Equal(finished) ||
		tool.ExitCode == nil || *tool.ExitCode != 0 || tool.Duration != 1250*time.Millisecond ||
		!json.Valid(tool.ArgumentsJSON) || !bytes.Contains(tool.ArgumentsJSON, []byte(`"command":"go test ./..."`)) ||
		!json.Valid(tool.ResultJSON) || !bytes.Contains(tool.ResultJSON, []byte(`"output":"ok"`)) {
		t.Fatalf("tool = %+v", tool)
	}
}

func TestProjectUnknownToolPreservesCompleteArgumentsAndResult(t *testing.T) {
	tool, err := projectTool(toolProjection{invocation: &protocol.ToolInvocation{
		Name: "mcp__calendar__create_event",
		Arguments: map[string]any{
			"calendar": "work", "guests": []any{"a@example.com", "b@example.com"},
			"metadata": map[string]any{"source": "lyra"},
		},
		Result: map[string]any{"eventId": "evt_123", "accepted": true},
	}, status: protocol.ItemStatusCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if tool.Kind != agent.ToolUnknown || tool.Name != "mcp__calendar__create_event" ||
		!bytes.Contains(tool.ArgumentsJSON, []byte(`"guests"`)) ||
		!bytes.Contains(tool.ArgumentsJSON, []byte(`"source":"lyra"`)) ||
		!bytes.Contains(tool.ResultJSON, []byte(`"eventId":"evt_123"`)) {
		t.Fatalf("unknown tool = %+v", tool)
	}
}

func TestProjectAssistantMessagePreservesInlineImages(t *testing.T) {
	data := []byte("generated image bytes")
	created := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	block, err := projectItem(protocol.Item{
		ID: "answer", RunID: "run_1", Status: protocol.ItemStatusCompleted, Type: protocol.ItemTypeAgentMessage,
		CreatedAt: created,
		Content: []protocol.ContentBlock{
			{Type: protocol.ContentBlockText, Text: "Generated chart"},
			{Type: protocol.ContentBlockImage, Mime: "image/png", Data: base64.StdEncoding.EncodeToString(data)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !block.CreatedAt.Equal(created) || block.Text != "Generated chart" || len(block.Images) != 1 || block.Images[0].Name != "image.png" ||
		block.Images[0].MIMEType != "image/png" || !bytes.Equal(block.Images[0].Data, data) || len(block.Attachments) != 0 {
		t.Fatalf("assistant block = %+v", block)
	}
}

func TestProjectItemPreservesReasoningAndCompactionMetadata(t *testing.T) {
	created := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	reasoning, err := projectItem(protocol.Item{
		ID: "reasoning", RunID: "run_1", Status: protocol.ItemStatusCompleted,
		Type: protocol.ItemTypeReasoning, CreatedAt: created, Redacted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reasoning.Redacted || !reasoning.CreatedAt.Equal(created) || reasoning.Text != "Reasoning redacted by provider." {
		t.Fatalf("reasoning = %+v", reasoning)
	}

	compaction, err := projectItem(protocol.Item{
		ID: "compaction", RunID: "run_1", Status: protocol.ItemStatusCompleted,
		Type: protocol.ItemTypeCompaction, CreatedAt: created, DroppedMessages: 17,
	})
	if err != nil {
		t.Fatal(err)
	}
	if compaction.DroppedMessages != 17 || !strings.Contains(compaction.Text, "17 messages") {
		t.Fatalf("compaction = %+v", compaction)
	}
}

func TestProjectRunUsagePreservesStepsAndPerModelAttribution(t *testing.T) {
	totalCost, modelCost := 0.4, 0.25
	usage := projectUsage(protocol.RunMetrics{
		Steps: 4, ActiveDurationMillis: 1250,
		Usage: &protocol.Usage{
			ModelUsage: protocol.ModelUsage{InputTokens: 100, CostUSD: &totalCost},
			ByModel: map[string]protocol.ModelUsage{
				"deepseek/v4": {InputTokens: 75, ReasoningTokens: 12, CostUSD: &modelCost},
			},
		},
	})
	model := usage.ByModel["deepseek/v4"]
	if usage.Steps != 4 || usage.Duration != 1250*time.Millisecond || usage.InputTokens != 100 ||
		model.InputTokens != 75 || model.ReasoningTokens != 12 || model.CostUSD == nil || *model.CostUSD != modelCost {
		t.Fatalf("usage = %+v", usage)
	}
	*model.CostUSD = 9
	if modelCost != 0.25 {
		t.Fatal("projected model cost aliases runtime usage")
	}
}

func TestProjectOutcomePreservesStructuredProblem(t *testing.T) {
	outcome, err := projectOutcome(protocol.SegmentOutcome{
		Type: protocol.SegmentFailed,
		Error: &protocol.ProblemData{
			Type: protocol.ProblemRateLimited, Detail: "quota exhausted",
			DocURL: "https://docs.example/rate-limit", RetryAfterSeconds: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != agent.OutcomeFailed || outcome.Description() != "quota exhausted" || outcome.Problem == nil ||
		outcome.Problem.RetryAfterSeconds != 2 || outcome.Problem.DocURL != "https://docs.example/rate-limit" {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestProjectRuntimeProblemPreservesEveryRecoveryShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		problem protocol.ProblemData
		assert  func(*testing.T, *failure.Problem)
	}{
		{
			name: "retry guidance",
			problem: protocol.ProblemData{
				Type: protocol.ProblemRateLimited, Detail: "quota exhausted",
				DocURL: "https://docs.example/rate-limit", RetryAfterSeconds: 2,
			},
			assert: func(t *testing.T, problem *failure.Problem) {
				if problem.Detail != "quota exhausted" || problem.DocURL == "" || problem.RetryAfterSeconds != 2 {
					t.Fatalf("retry problem = %+v", problem)
				}
			},
		},
		{
			name: "capability requirements",
			problem: protocol.ProblemData{
				Type:                 protocol.ErrCapabilityNotNeg.Error(),
				RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementRuntimeTopic, Name: "files.changed"}},
			},
			assert: func(t *testing.T, problem *failure.Problem) {
				if len(problem.RequiredCapabilities) != 1 || problem.RequiredCapabilities[0].Kind != failure.RequirementRuntimeTopic || problem.RequiredCapabilities[0].Name != "files.changed" {
					t.Fatalf("capability problem = %+v", problem)
				}
			},
		},
		{
			name: "active run",
			problem: protocol.ProblemData{
				Type:      protocol.ErrSessionHasActiveRun.Error(),
				ActiveRun: &protocol.ActiveRunRef{RunID: "run_1", Status: protocol.RunStatusWaiting},
			},
			assert: func(t *testing.T, problem *failure.Problem) {
				if problem.ActiveRun == nil || problem.ActiveRun.RunID != "run_1" || problem.ActiveRun.Status != "waiting" {
					t.Fatalf("active-run problem = %+v", problem)
				}
			},
		},
		{
			name: "field errors",
			problem: protocol.ProblemData{
				Type:   protocol.ErrInvalidParams.Error(),
				Errors: []protocol.FieldError{{Field: "provider", Detail: "is unknown"}},
			},
			assert: func(t *testing.T, problem *failure.Problem) {
				if len(problem.Errors) != 1 || problem.Errors[0].Field != "provider" || problem.Errors[0].Detail != "is unknown" {
					t.Fatalf("field problem = %+v", problem)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			projected := projectRuntimeProblem(&test.problem)
			if projected == nil {
				t.Fatal("projectRuntimeProblem returned nil")
			}
			if err := projected.Validate(); err != nil {
				t.Fatal(err)
			}
			test.assert(t, projected)
		})
	}
}

func TestProjectToolRecognizesRootRunCancellation(t *testing.T) {
	tool, err := projectTool(toolProjection{
		invocation: &protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{"command": "sleep 30"}},
		status:     protocol.ItemStatusIncomplete,
		problem:    &protocol.ProblemData{Type: protocol.ProblemToolCanceled, Detail: "run canceled"},
	})
	if err != nil {
		t.Fatalf("projectTool: %v", err)
	}
	if tool.Status != agent.ToolCanceled || tool.Output != "run canceled" ||
		tool.Problem == nil || tool.Problem.Type != "tool_canceled" {
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

func TestCompletedQuestionPreservesAcceptedAnswers(t *testing.T) {
	t.Parallel()

	answers := [][]string{{"safe"}}
	block, err := projectItem(protocol.Item{
		ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusCompleted,
		Type: protocol.ItemTypeQuestion, Question: &protocol.Question{
			Fields: []protocol.QuestionField{{
				Prompt: "Choose a strategy", Type: protocol.QuestionFieldChoice,
				Options: []protocol.QuestionOption{{Label: "safe"}, {Label: "fast"}},
			}},
			Answers: answers,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if block.Question == nil || !block.Question.Answered() || !reflect.DeepEqual(block.Question.Answers, answers) {
		t.Fatalf("completed question = %+v", block.Question)
	}
	answers[0][0] = "mutated"
	if block.Question.Answers[0][0] != "safe" {
		t.Fatal("question projection aliases runtime answer storage")
	}
}

func TestCompletedQuestionCannotReenterTheInterruptChannel(t *testing.T) {
	t.Parallel()

	_, err := projectInteraction(protocol.Interrupt{
		ItemID: "item_1", RunID: "run_1", Type: protocol.InterruptQuestion,
		Payload: &protocol.InterruptPayload{Question: &protocol.Question{
			Fields:  []protocol.QuestionField{{Prompt: "Target", Type: protocol.QuestionFieldText}},
			Answers: [][]string{{"linux"}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "already has accepted answers") {
		t.Fatalf("answered question interrupt error = %v", err)
	}
}

func TestApprovalInterruptPreservesCompleteToolArguments(t *testing.T) {
	interaction, err := projectInteraction(protocol.Interrupt{
		ItemID: "tool_1", RunID: "run_1", Type: protocol.InterruptApproval,
		Payload: &protocol.InterruptPayload{
			Tool: &protocol.ToolInvocation{
				Name: "mcp__calendar__create_event",
				Arguments: map[string]any{
					"calendar": "work", "metadata": map[string]any{"source": "approval"},
				},
			},
			Risk: protocol.ApprovalRiskHigh, Reason: "creates a shared event", Rememberable: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	approval := interaction.(agent.Approval)
	if approval.Tool == nil || approval.Tool.Name != "mcp__calendar__create_event" ||
		!bytes.Contains(approval.Tool.ArgumentsJSON, []byte(`"source":"approval"`)) ||
		approval.Risk != agent.ApprovalRiskHigh || approval.Detail != "creates a shared event" || !approval.Rememberable {
		t.Fatalf("approval = %+v", approval)
	}
}

func TestProjectEventPreservesEphemeralFramesAndClassifiesStreams(t *testing.T) {
	step, contextTokens, cost := 3, int64(8_192), 0.25
	at := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	project := func(eventID string, event protocol.StreamEvent) (agent.RunEvent, bool, error) {
		return projectEvent(protocol.RunEvent{
			EventID: eventID, RunID: "run_1", SegmentID: "segment_1", Timestamp: at, Event: event,
		})
	}
	progressEvent, include, err := project("progress", protocol.StreamEvent{
		Type: protocol.StreamSegmentProgress,
		Progress: &protocol.RunProgress{
			Step: &step, ContextTokens: &contextTokens, Activity: "thinking",
			Usage: &protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: 12, CostUSD: &cost}},
		},
	})
	if err != nil || !include {
		t.Fatalf("progress = (include %v, error %v)", include, err)
	}
	progress, ok := progressEvent.Event.(agent.RunProgress)
	if !ok || progress.Step == nil || *progress.Step != step || progress.ContextTokens == nil ||
		*progress.ContextTokens != contextTokens || progress.Usage == nil || progress.Usage.InputTokens != 12 ||
		progress.Usage.CostUSD == nil || *progress.Usage.CostUSD != cost || progress.Activity != "thinking" {
		t.Fatalf("projected progress = %#v", progressEvent.Event)
	}

	arguments, include, err := project("arguments", protocol.StreamEvent{
		Type: protocol.StreamItemDelta, ItemID: "tool_1",
		Delta: &protocol.ItemDelta{Type: protocol.DeltaToolArguments, ArgumentsTextDelta: `{"path":"/tmp`},
	})
	if err != nil || !include || arguments.Event != (agent.ToolArgumentsDelta{BlockID: "tool_1", Text: `{"path":"/tmp`}) {
		t.Fatalf("tool arguments = %#v, include %v, error %v", arguments.Event, include, err)
	}

	contentIndex := 2
	content, include, err := project("content", protocol.StreamEvent{
		Type: protocol.StreamItemDelta, ItemID: "answer",
		Delta: &protocol.ItemDelta{Type: protocol.DeltaContent, Index: &contentIndex, Text: "third block"},
	})
	if err != nil || !include {
		t.Fatalf("content = (include %v, error %v)", include, err)
	}
	delta, ok := content.Event.(agent.BlockDelta)
	if !ok || delta.BlockID != "answer" || delta.Text != "third block" || delta.ContentIndex == nil || *delta.ContentIndex != contentIndex {
		t.Fatalf("content delta = %#v", content.Event)
	}
	*delta.ContentIndex = 9
	if contentIndex != 2 {
		t.Fatal("projected content index aliases the runtime event")
	}

	customEvent, include, err := project("custom", protocol.StreamEvent{
		Type: protocol.StreamCustom, Name: "vendor.trace", Payload: map[string]any{"span": "abc", "sampled": true},
	})
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

func TestProjectEventConsumesAuthoritativeItemAndStateFrames(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		event  protocol.StreamEvent
		assert func(*testing.T, agent.RunEvent)
	}{
		{
			name: "item started",
			event: protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &protocol.Item{
				ID: "answer", RunID: "run_1", Status: protocol.ItemStatusRunning, Type: protocol.ItemTypeAgentMessage,
			}},
			assert: func(t *testing.T, event agent.RunEvent) {
				started, ok := event.Event.(agent.BlockStarted)
				if !ok || started.Block.ID != "answer" || started.Block.Status != agent.BlockStatusRunning {
					t.Fatalf("item.started = %#v", event.Event)
				}
			},
		},
		{
			name: "item completed",
			event: protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &protocol.Item{
				ID: "answer", RunID: "run_1", Status: protocol.ItemStatusCompleted, Type: protocol.ItemTypeAgentMessage,
				Content: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "done"}},
			}},
			assert: func(t *testing.T, event agent.RunEvent) {
				completed, ok := event.Event.(agent.BlockCompleted)
				if !ok || completed.Block.Text != "done" || completed.Block.Status != agent.BlockStatusCompleted {
					t.Fatalf("item.completed = %#v", event.Event)
				}
			},
		},
		{
			name: "state snapshot",
			event: protocol.StreamEvent{Type: protocol.StreamStateSnapshot, State: &protocol.StateSnapshot{
				Type: protocol.StatePlan, SessionID: "session_1", Revision: 2, UpdatedAt: at,
				Plan: []protocol.PlanSnapshot{{ID: "step_1", Description: "verify", Status: protocol.PlanStatusInProgress}},
			}},
			assert: func(t *testing.T, event agent.RunEvent) {
				plan, ok := event.Event.(agent.PlanChanged)
				if !ok || plan.Revision != 2 || len(plan.Items) != 1 || plan.Items[0].Title != "verify" || plan.Items[0].Status != agent.PlanActive {
					t.Fatalf("state.snapshot = %#v", event.Event)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			event, included, err := projectEvent(protocol.RunEvent{
				EventID: "event_1", RunID: "run_1", SegmentID: "segment_1", Timestamp: at, Event: test.event,
			})
			if err != nil || !included {
				t.Fatalf("projectEvent = (%+v, %v, %v)", event, included, err)
			}
			if event.EventID != "event_1" || event.RunID != "run_1" || event.SegmentID != "segment_1" || !event.At.Equal(at) {
				t.Fatalf("event envelope = %+v", event)
			}
			test.assert(t, event)
		})
	}
}

func TestProjectEventRejectsMalformedEnvelopeBeforeStreaming(t *testing.T) {
	t.Parallel()

	for _, event := range []protocol.RunEvent{
		{
			EventID: "event_1", RunID: "run_1", SegmentID: "segment_1",
			Event: protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &protocol.Item{
				ID: "answer", RunID: "run_1", Status: protocol.ItemStatusCompleted, Type: protocol.ItemTypeAgentMessage,
			}},
		},
		{
			EventID: "event_1", RunID: "run_1", SegmentID: "segment_1", Timestamp: time.Now(),
			Event: protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &protocol.Item{
				ID: "answer", RunID: "another_run", Status: protocol.ItemStatusCompleted, Type: protocol.ItemTypeAgentMessage,
			}},
		},
	} {
		_, included, err := projectEvent(event)
		if err == nil || included {
			t.Fatalf("malformed event = (included %v, error %v)", included, err)
		}
	}
}

func TestRunProfileAcceptsSubagentTrees(t *testing.T) {
	_, err := projectRunContract(protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
	})
	if err != nil {
		t.Fatalf("projectRunContract: %v", err)
	}
}

func TestProjectChildRunPreservesLineage(t *testing.T) {
	created := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	projected, err := projectRun(protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_child", SessionID: "ses_1", Status: protocol.RunStatusRunning,
			SpawnedByItemID: "item_delegate", ParentRunID: "run_root", RootRunID: "run_root",
			CreatedAt: created,
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
	wantContract := &agent.RunContract{
		RequiredFeatures: []agent.RunFeature{agent.RunFeatureSubagents},
		InteractionKinds: []agent.InteractionKind{agent.InteractionApproval, agent.InteractionQuestion},
	}
	if projected.Lineage != want || !projected.CreatedAt.Equal(created) || !reflect.DeepEqual(projected.Contract, wantContract) {
		t.Fatalf("projected run = %+v", projected)
	}
}

func TestProjectTreeStreamRetainsProducerAndStreamSegments(t *testing.T) {
	source := func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{
			RunID: "run_root", SegmentID: "seg_root", EventID: "evt_suspend", Timestamp: time.Now(),
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
			Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
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
