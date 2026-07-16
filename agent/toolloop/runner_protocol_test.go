package toolloop_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type runnerTool struct {
	definition chat.ToolDefinition
	call       func(context.Context, string) (string, error)
	concurrent func(string) (string, bool)
}

func (t *runnerTool) Definition() chat.ToolDefinition { return t.definition }

func (t *runnerTool) Call(ctx context.Context, arguments string) (string, error) {
	if t.call == nil {
		return "", nil
	}
	return t.call(ctx, arguments)
}

func (t *runnerTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if t.concurrent == nil {
		return "", false
	}
	return t.concurrent(arguments)
}

type scriptedModel struct {
	calls int
	call  func(int, *chat.Request) (*chat.Response, error)
}

func (m *scriptedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.calls++
	return m.call(m.calls, request)
}

func TestRunnerMultiRoundAndDefensiveEvents(t *testing.T) {
	var toolCalls int
	lookup := newRunnerTool("lookup", func(_ context.Context, arguments string) (string, error) {
		toolCalls++
		if arguments != `{"q":"lynx"}` {
			t.Fatalf("tool arguments = %q", arguments)
		}
		return "found", nil
	})
	registry := newRunnerRegistry(t, lookup)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		switch round {
		case 1:
			if got := request.Messages[0].Text(); got != "hello" {
				t.Fatalf("first request text = %q; event mutation leaked", got)
			}
			return runnerToolResponse(chat.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{"q":"lynx"}`}), nil
		case 2:
			if len(request.Messages) != 3 {
				t.Fatalf("continuation has %d messages, want 3", len(request.Messages))
			}
			result := request.Messages[2].Parts[0].ToolResult
			if result == nil || result.Result != "found" || result.IsError {
				t.Fatalf("continuation result = %#v", result)
			}
			return runnerTextResponse("done"), nil
		default:
			t.Fatalf("unexpected model round %d", round)
			return nil, nil
		}
	}}
	runner := newRunner(t, model, toolloop.Config{})
	request := newRunnerRequest(t, registry)

	var events []toolloop.Event
	for event, err := range runner.Run(context.Background(), request, registry) {
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if err := event.Validate(); err != nil {
			t.Fatalf("invalid event: %v", err)
		}
		events = append(events, event)
		switch event.Kind {
		case toolloop.EventModelRequest:
			event.Request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("mutated"))
		case toolloop.EventModelResponse:
			if !event.Final && event.Response.First().Message.Parts[0].ToolCall != nil {
				event.Response.First().Message.Parts[0].ToolCall.Name = "mutated"
			}
		}
	}

	wantKinds := []toolloop.EventKind{
		toolloop.EventModelRequest,
		toolloop.EventModelResponse,
		toolloop.EventToolCall,
		toolloop.EventToolResult,
		toolloop.EventModelRequest,
		toolloop.EventModelResponse,
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event kinds = %v, want %v", got, wantKinds)
	}
	if model.calls != 2 || toolCalls != 1 {
		t.Fatalf("calls = model %d, tool %d", model.calls, toolCalls)
	}
	if events[1].Final || events[3].Final || !events[5].Final {
		t.Fatalf("final markers = response %v, tool %v, final response %v", events[1].Final, events[3].Final, events[5].Final)
	}
	if len(request.Messages) != 1 || request.Messages[0].Text() != "hello" {
		t.Fatalf("Run mutated original request: %#v", request.Messages)
	}
}

func TestRunnerTurnsOrdinaryAndUnknownToolErrorsIntoFeedback(t *testing.T) {
	broken := newRunnerTool("broken", func(context.Context, string) (string, error) {
		return "", errors.New("disk unavailable")
	})
	registry := newRunnerRegistry(t, broken)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "broken-1", Name: "broken", Arguments: `{}`},
				chat.ToolCall{ID: "missing-1", Name: "missing", Arguments: `{}`},
			), nil
		}
		parts := request.Messages[len(request.Messages)-1].Parts
		if len(parts) != 2 {
			t.Fatalf("tool results = %d, want 2", len(parts))
		}
		for i := range parts {
			if parts[i].ToolResult == nil || !parts[i].ToolResult.IsError {
				t.Fatalf("parts[%d] = %#v, want error result", i, parts[i])
			}
		}
		if got := parts[0].ToolResult.Result; got != `error: tool "broken" failed: disk unavailable` {
			t.Fatalf("ordinary error result = %q", got)
		}
		if got := parts[1].ToolResult.Result; got != `error: tool "missing" is not available` {
			t.Fatalf("unknown error result = %q", got)
		}
		return runnerTextResponse("recovered"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	events, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(events) != 8 || !events[len(events)-1].Final {
		t.Fatalf("events = %#v", events)
	}
}

func TestRunnerDirectPolicy(t *testing.T) {
	t.Run("all direct", func(t *testing.T) {
		var calls int
		first := toolloop.Direct(newRunnerTool("first", func(context.Context, string) (string, error) {
			calls++
			return "one", nil
		}))
		second := toolloop.Direct(newRunnerTool("second", func(context.Context, string) (string, error) {
			calls++
			return "two", nil
		}))
		registry := newRunnerRegistry(t, first, second)
		model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
			return runnerToolResponse(
				chat.ToolCall{ID: "1", Name: "first", Arguments: `{}`},
				chat.ToolCall{ID: "2", Name: "second", Arguments: `{}`},
			), nil
		}}
		runner := newRunner(t, model, toolloop.Config{})
		events, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
		if err != nil {
			t.Fatal(err)
		}
		if model.calls != 1 || calls != 2 {
			t.Fatalf("calls = model %d, tools %d", model.calls, calls)
		}
		if len(events) != 6 || events[3].Final || !events[5].Final || events[5].Kind != toolloop.EventToolResult {
			t.Fatalf("direct events = %#v", events)
		}
	})

	t.Run("mixed round continues", func(t *testing.T) {
		direct := toolloop.Direct(newRunnerTool("direct", nil))
		normal := newRunnerTool("normal", nil)
		registry := newRunnerRegistry(t, direct, normal)
		model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
			if round == 1 {
				return runnerToolResponse(
					chat.ToolCall{ID: "1", Name: "direct", Arguments: `{}`},
					chat.ToolCall{ID: "2", Name: "normal", Arguments: `{}`},
				), nil
			}
			return runnerTextResponse("done"), nil
		}}
		runner := newRunner(t, model, toolloop.Config{})
		events, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
		if err != nil {
			t.Fatal(err)
		}
		if model.calls != 2 || !events[len(events)-1].Final {
			t.Fatalf("model calls = %d, events = %#v", model.calls, events)
		}
	})
}

func TestRunnerPauseAndResumeDoesNotRepeatCompletedWork(t *testing.T) {
	var completedCalls, approvalAttempts, approvedEffects int
	first := newRunnerTool("first", func(context.Context, string) (string, error) {
		completedCalls++
		return "first done", nil
	})
	approval := newRunnerTool("approval", func(ctx context.Context, _ string) (string, error) {
		approvalAttempts++
		resume, ok := toolloop.ResumeFromContext(ctx)
		if !ok {
			return "", approvalPause()
		}
		if resume.ID != "approve-1" || string(resume.Input) != `"approved"` {
			t.Fatalf("resume = %#v", resume)
		}
		approvedEffects++
		return "approved", nil
	})
	registry := newRunnerRegistry(t, first, approval)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "first-1", Name: "first", Arguments: `{}`},
				chat.ToolCall{ID: "approval-1", Name: "approval", Arguments: `{}`},
			), nil
		}
		parts := request.Messages[len(request.Messages)-1].Parts
		if len(parts) != 2 || parts[0].ToolResult.Result != "first done" || parts[1].ToolResult.Result != "approved" {
			t.Fatalf("resumed results = %#v", parts)
		}
		return runnerTextResponse("finished"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	firstEvents, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	pauseEvent := firstEvents[len(firstEvents)-1]
	checkpoint := pauseEvent.Pause.Checkpoint
	if pauseEvent.Kind != toolloop.EventPause ||
		checkpoint.NextResult != 1 ||
		len(checkpoint.CallStates) != 2 ||
		checkpoint.CallStates[0].Status != toolloop.CallCompleted ||
		checkpoint.CallStates[1].Status != toolloop.CallPaused {
		t.Fatalf("pause event = %#v", pauseEvent)
	}
	if model.calls != 1 || completedCalls != 1 || approvalAttempts != 1 || approvedEffects != 0 {
		t.Fatalf("before resume: model %d, completed %d, attempts %d, effects %d", model.calls, completedCalls, approvalAttempts, approvedEffects)
	}
	body, err := json.Marshal(pauseEvent)
	if err != nil {
		t.Fatalf("Marshal pause: %v", err)
	}
	var restored toolloop.Event
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatalf("Unmarshal pause: %v", err)
	}

	resume := toolloop.Resume{ID: "approve-1", Input: json.RawMessage(`"approved"`)}
	resumedEvents, err := collectRunnerEvents(runner.Resume(context.Background(), restored.Pause.Checkpoint, registry, resume))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	wantKinds := []toolloop.EventKind{
		toolloop.EventResume,
		toolloop.EventToolCall,
		toolloop.EventToolResult,
		toolloop.EventModelRequest,
		toolloop.EventModelResponse,
	}
	if got := eventKinds(resumedEvents); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("resume kinds = %v, want %v", got, wantKinds)
	}
	if model.calls != 2 || completedCalls != 1 || approvalAttempts != 2 || approvedEffects != 1 {
		t.Fatalf("after resume: model %d, completed %d, attempts %d, effects %d", model.calls, completedCalls, approvalAttempts, approvedEffects)
	}
}

func TestRunnerControlFlowAndModelFailures(t *testing.T) {
	sentinel := errors.New("fatal")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "abort", err: fmt.Errorf("wrapped: %w", &toolloop.AbortError{Err: sentinel}), want: sentinel},
		{name: "cancel", err: fmt.Errorf("wrapped: %w", context.Canceled), want: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "invalid pause", err: &toolloop.PauseError{ID: "missing-reason"}, want: toolloop.ErrInvalidControlFlow},
		{name: "invalid abort", err: &toolloop.AbortError{}, want: toolloop.ErrInvalidControlFlow},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := newRunnerRegistry(t, newRunnerTool("stop", func(context.Context, string) (string, error) {
				return "", test.err
			}))
			model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
				return runnerToolResponse(chat.ToolCall{ID: "stop-1", Name: "stop", Arguments: `{}`}), nil
			}}
			runner := newRunner(t, model, toolloop.Config{})
			_, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
			if !errors.Is(err, test.want) {
				t.Fatalf("Run error = %v, want errors.Is %v", err, test.want)
			}
		})
	}

	t.Run("model error", func(t *testing.T) {
		modelErr := errors.New("provider failed")
		model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) { return nil, modelErr }}
		runner := newRunner(t, model, toolloop.Config{})
		_, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, nil), nil))
		if !errors.Is(err, modelErr) {
			t.Fatalf("Run error = %v", err)
		}
	})

	t.Run("nil response", func(t *testing.T) {
		model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) { return nil, nil }}
		runner := newRunner(t, model, toolloop.Config{})
		_, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, nil), nil))
		if err == nil {
			t.Fatal("Run unexpectedly succeeded")
		}
	})

	t.Run("invalid response", func(t *testing.T) {
		model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
			return &chat.Response{Choices: []chat.Choice{{Index: -1}}}, nil
		}}
		runner := newRunner(t, model, toolloop.Config{})
		_, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, nil), nil))
		if !errors.Is(err, chat.ErrInvalidResponse) {
			t.Fatalf("Run error = %v", err)
		}
	})
}

func TestRunnerHandlesHallucinatedToolWithoutResolver(t *testing.T) {
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(chat.ToolCall{ID: "missing-1", Name: "missing", Arguments: `{}`}), nil
		}
		result := request.Messages[len(request.Messages)-1].Parts[0].ToolResult
		if result == nil || !result.IsError {
			t.Fatalf("missing tool result = %#v", result)
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	if _, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, nil), nil)); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunnerRoundLimitLazinessAndEarlyStop(t *testing.T) {
	toolCalls := 0
	registry := newRunnerRegistry(t, newRunnerTool("again", func(context.Context, string) (string, error) {
		toolCalls++
		return "again", nil
	}))
	model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
		return runnerToolResponse(chat.ToolCall{ID: fmt.Sprintf("call-%d", round), Name: "again", Arguments: `{}`}), nil
	}}
	runner := newRunner(t, model, toolloop.Config{MaxRounds: 2})
	sequence := runner.Run(context.Background(), newRunnerRequest(t, registry), registry)
	if model.calls != 0 || toolCalls != 0 {
		t.Fatal("Run was not lazy")
	}
	_, err := collectRunnerEvents(sequence)
	if !errors.Is(err, toolloop.ErrRoundLimit) || model.calls != 2 || toolCalls != 2 {
		t.Fatalf("limit result = %v, model %d, tools %d", err, model.calls, toolCalls)
	}

	stoppedModel := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
		t.Fatal("model called after consumer stopped at request event")
		return nil, nil
	}}
	stoppedRunner := newRunner(t, stoppedModel, toolloop.Config{})
	stoppedRunner.Run(context.Background(), newRunnerRequest(t, nil), nil)(func(event toolloop.Event, err error) bool {
		if err != nil || event.Kind != toolloop.EventModelRequest {
			t.Fatalf("first yield = %#v, %v", event, err)
		}
		return false
	})
	if stoppedModel.calls != 0 {
		t.Fatalf("stopped model calls = %d", stoppedModel.calls)
	}
}

func TestRunnerRejectsInvalidConfigRunAndResume(t *testing.T) {
	if _, err := toolloop.NewRunner(nil, toolloop.Config{}); !errors.Is(err, toolloop.ErrInvalidConfig) {
		t.Fatalf("nil model error = %v", err)
	}
	var typedNilModel *scriptedModel
	if _, err := toolloop.NewRunner(typedNilModel, toolloop.Config{}); !errors.Is(err, toolloop.ErrInvalidConfig) {
		t.Fatalf("typed nil model error = %v", err)
	}
	validModel := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) { return runnerTextResponse("ok"), nil }}
	if _, err := toolloop.NewRunner(validModel, toolloop.Config{MaxRounds: -1}); !errors.Is(err, toolloop.ErrInvalidConfig) {
		t.Fatalf("negative rounds error = %v", err)
	}
	if _, err := toolloop.NewRunner(validModel, toolloop.Config{MaxConcurrentCalls: -1}); !errors.Is(err, toolloop.ErrInvalidConfig) {
		t.Fatalf("negative concurrency error = %v", err)
	}
	runner := newRunner(t, validModel, toolloop.Config{})
	var nilContext context.Context
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collectRunnerEvents(runner.Run(canceled, newRunnerRequest(t, nil), nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Run error = %v", err)
	}

	for _, test := range []struct {
		name string
		seq  iter.Seq2[toolloop.Event, error]
	}{
		{name: "nil context", seq: runner.Run(nilContext, newRunnerRequest(t, nil), nil)},
		{name: "nil request", seq: runner.Run(context.Background(), nil, nil)},
		{name: "invalid request", seq: runner.Run(context.Background(), &chat.Request{}, nil)},
		{name: "zero runner", seq: (*toolloop.Runner)(nil).Run(context.Background(), newRunnerRequest(t, nil), nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := collectRunnerEvents(test.seq)
			if !errors.Is(err, toolloop.ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	checkpoint := protocolCheckpoint(t)
	registry := protocolRegistry(t)
	for _, test := range []struct {
		name       string
		checkpoint *toolloop.Checkpoint
		resolver   toolloop.ToolResolver
		resume     toolloop.Resume
	}{
		{name: "nil checkpoint", resolver: registry, resume: toolloop.Resume{ID: "approval-1"}},
		{name: "empty resume", checkpoint: checkpoint, resolver: registry},
		{name: "mismatched ID", checkpoint: checkpoint, resolver: registry, resume: toolloop.Resume{ID: "other"}},
		{name: "missing resolver", checkpoint: checkpoint, resume: toolloop.Resume{ID: "approval-1"}},
		{name: "nil context", checkpoint: checkpoint, resolver: registry, resume: toolloop.Resume{ID: "approval-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.name == "nil context" {
				ctx = nil
			}
			_, err := collectRunnerEvents(runner.Resume(ctx, test.checkpoint, test.resolver, test.resume))
			if !errors.Is(err, toolloop.ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	serialRunner := newRunner(t, validModel, toolloop.Config{MaxConcurrentCalls: 1})
	_, err := collectRunnerEvents(serialRunner.Resume(
		context.Background(),
		checkpoint,
		registry,
		toolloop.Resume{ID: "approval-1", Input: json.RawMessage(`"approved"`)},
	))
	if !errors.Is(err, toolloop.ErrInvalidInput) {
		t.Fatalf("concurrency-policy mismatch error = %v", err)
	}
}

func TestRunnerRejectsAmbiguousToolBranches(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *chat.Response
	}{
		{
			name: "second choice tool call",
			response: &chat.Response{Choices: []chat.Choice{
				{Index: 0, Message: messagePointer(chat.NewAssistantMessage(chat.NewTextPart("first")))},
				{Index: 1, Message: messagePointer(chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "2", Name: "tool"})))},
			}},
		},
		{
			name: "duplicate call ID",
			response: runnerToolResponse(
				chat.ToolCall{ID: "same", Name: "tool"},
				chat.ToolCall{ID: "same", Name: "tool"},
			),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) { return test.response, nil }}
			runner := newRunner(t, model, toolloop.Config{})
			_, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, nil), nil))
			if !errors.Is(err, toolloop.ErrAmbiguousToolCalls) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCheckpointValidationAndAtomicJSON(t *testing.T) {
	valid := protocolCheckpoint(t)
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid checkpoint: %v", err)
	}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded toolloop.Checkpoint
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded, *valid) {
		t.Fatalf("decoded = %#v, want %#v", decoded, *valid)
	}
	unknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"future":true}`)...)
	if err := json.Unmarshal(unknown, &decoded); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
		t.Fatalf("unknown field error = %v", err)
	}
	if !reflect.DeepEqual(decoded, *valid) {
		t.Fatal("unknown-field failure mutated checkpoint")
	}

	original := decoded
	if err := json.Unmarshal([]byte(`{"id":"broken"}`), &decoded); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
		t.Fatalf("invalid Unmarshal error = %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("failed Unmarshal mutated checkpoint")
	}
	if err := json.Unmarshal([]byte(`{`), &decoded); err == nil {
		t.Fatalf("malformed Unmarshal error = %v", err)
	}
	var nilCheckpoint *toolloop.Checkpoint
	if err := nilCheckpoint.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
		t.Fatalf("nil receiver error = %v", err)
	}

	for _, mutate := range []func(*toolloop.Checkpoint){
		func(c *toolloop.Checkpoint) { c.SchemaVersion = 1 },
		func(c *toolloop.Checkpoint) { c.ID = "" },
		func(c *toolloop.Checkpoint) { c.Round = 0 },
		func(c *toolloop.Checkpoint) { c.MaxConcurrentCalls = 0 },
		func(c *toolloop.Checkpoint) { c.Request = nil },
		func(c *toolloop.Checkpoint) { c.Request = &chat.Request{} },
		func(c *toolloop.Checkpoint) { c.Response = nil },
		func(c *toolloop.Checkpoint) { c.Response = &chat.Response{Choices: []chat.Choice{{Index: -1}}} },
		func(c *toolloop.Checkpoint) { c.Response = runnerTextResponse("no calls") },
		func(c *toolloop.Checkpoint) { c.NextResult = -1 },
		func(c *toolloop.Checkpoint) { c.NextResult = 1 },
		func(c *toolloop.Checkpoint) { c.CallStates = nil },
		func(c *toolloop.Checkpoint) { c.CallStates[0].Status = "future" },
		func(c *toolloop.Checkpoint) {
			c.CallStates[0] = toolloop.CallCheckpoint{
				Status: toolloop.CallCompleted,
				Result: &chat.ToolResult{ID: "call-1", Name: "lookup", Result: "done"},
			}
		},
	} {
		copy := cloneProtocolCheckpoint(t, valid)
		mutate(&copy)
		if err := copy.Validate(); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
			t.Fatalf("Validate(%#v) = %v", copy, err)
		}
		if _, err := json.Marshal(copy); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
			t.Fatalf("Marshal(%#v) = %v", copy, err)
		}
	}

	twoCalls := protocolCheckpoint(t)
	twoCalls.Response = runnerToolResponse(
		chat.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{}`},
		chat.ToolCall{ID: "call-2", Name: "lookup", Arguments: `{}`},
	)
	twoCalls.CallStates = []toolloop.CallCheckpoint{
		{
			Status: toolloop.CallCompleted,
			Result: &chat.ToolResult{ID: "call-1", Name: "lookup", Result: "done"},
		},
		{
			Status: toolloop.CallPaused,
			Pending: &toolloop.PendingCall{
				ID:           "approval-1",
				Reason:       "wait",
				Prompt:       json.RawMessage(`"approve?"`),
				ResumeSchema: json.RawMessage(`{"type":"string"}`),
			},
		},
	}
	twoCalls.NextResult = 1
	for _, result := range []chat.ToolResult{
		{ID: "", Name: "lookup", Result: "done"},
		{ID: "wrong", Name: "lookup", Result: "done"},
	} {
		twoCalls.CallStates[0].Result = &result
		if err := twoCalls.Validate(); !errors.Is(err, toolloop.ErrInvalidCheckpoint) {
			t.Fatalf("invalid completed result error = %v", err)
		}
	}

}

func cloneProtocolCheckpoint(t *testing.T, checkpoint *toolloop.Checkpoint) toolloop.Checkpoint {
	t.Helper()
	body, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatalf("marshal checkpoint clone: %v", err)
	}
	var clone toolloop.Checkpoint
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatalf("unmarshal checkpoint clone: %v", err)
	}
	return clone
}

func TestRuntimePolicyAndControlValues(t *testing.T) {
	if toolloop.Direct(nil) != nil {
		t.Fatal("Direct(nil) must return nil")
	}
	var typedNil *runnerTool
	if toolloop.Direct(typedNil) != nil {
		t.Fatal("Direct(typed nil) must return nil")
	}

	pause := (*toolloop.PauseError)(nil)
	if pause.Error() == "" {
		t.Fatal("nil PauseError has empty Error")
	}
	if got := approvalPause().Error(); got == "" {
		t.Fatal("PauseError has empty Error")
	}
	abort := (*toolloop.AbortError)(nil)
	if abort.Error() == "" || abort.Unwrap() != nil {
		t.Fatalf("nil AbortError = %q, unwrap %v", abort.Error(), abort.Unwrap())
	}
	var nilContext context.Context
	if _, ok := toolloop.ResumeFromContext(nilContext); ok {
		t.Fatal("nil context unexpectedly carried resume")
	}
}

func approvalPause() *toolloop.PauseError {
	return &toolloop.PauseError{
		ID:           "approve-1",
		Reason:       "approval required",
		Prompt:       json.RawMessage(`"approve?"`),
		ResumeSchema: json.RawMessage(`{"type":"string"}`),
	}
}

func newRunnerTool(name string, call func(context.Context, string) (string, error)) *runnerTool {
	return &runnerTool{
		definition: chat.ToolDefinition{
			Name:        name,
			Description: name + " tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		call: call,
	}
}

func newConcurrentRunnerTool(
	name string,
	key string,
	call func(context.Context, string) (string, error),
) *runnerTool {
	tool := newRunnerTool(name, call)
	tool.concurrent = func(string) (string, bool) { return key, true }
	return tool
}

func newRunnerRegistry(t *testing.T, values ...tools.Tool) *tools.Registry {
	t.Helper()
	registry, err := tools.NewRegistry(values...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func newRunnerRequest(t *testing.T, registry *tools.Registry) *chat.Request {
	t.Helper()
	request := protocolRequest(t)
	if registry != nil {
		request.Tools = registry.Definitions()
	}
	return request
}

func newRunner(t *testing.T, model chat.Model, config toolloop.Config) *toolloop.Runner {
	t.Helper()
	runner, err := toolloop.NewRunner(model, config)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return runner
}

func runnerToolResponse(calls ...chat.ToolCall) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for i := range calls {
		parts[i] = chat.NewToolCallPart(calls[i])
	}
	return &chat.Response{Choices: []chat.Choice{{
		Index:        0,
		Message:      messagePointer(chat.NewAssistantMessage(parts...)),
		FinishReason: chat.FinishReasonToolCalls,
	}}}
}

func runnerTextResponse(text string) *chat.Response {
	return &chat.Response{Choices: []chat.Choice{{
		Index:        0,
		Message:      messagePointer(chat.NewAssistantMessage(chat.NewTextPart(text))),
		FinishReason: chat.FinishReasonStop,
	}}}
}

func collectRunnerEvents(sequence iter.Seq2[toolloop.Event, error]) ([]toolloop.Event, error) {
	var events []toolloop.Event
	for event, err := range sequence {
		if err != nil {
			return events, err
		}
		if validationErr := event.Validate(); validationErr != nil {
			return events, validationErr
		}
		events = append(events, event)
	}
	return events, nil
}

func eventKinds(events []toolloop.Event) []toolloop.EventKind {
	kinds := make([]toolloop.EventKind, len(events))
	for i := range events {
		kinds[i] = events[i].Kind
	}
	return kinds
}
