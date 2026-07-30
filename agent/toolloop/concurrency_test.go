package toolloop_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

func TestRunnerExecutesIndependentCallsConcurrentlyAndPublishesInCallOrder(t *testing.T) {
	started := make(chan string, 2)
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	finishedSecond := make(chan struct{})

	first := newConcurrentRunnerTool("first", "", func(context.Context, string) (string, error) {
		started <- "first"
		<-releaseFirst
		return "one", nil
	})
	second := newConcurrentRunnerTool("second", "", func(context.Context, string) (string, error) {
		started <- "second"
		<-releaseSecond
		close(finishedSecond)
		return "two", nil
	})
	registry := newRunnerRegistry(t, first, second)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "call-1", Name: "first", Arguments: `{}`},
				chat.ToolCall{ID: "call-2", Name: "second", Arguments: `{}`},
			), nil
		}
		parts := request.Messages[len(request.Messages)-1].Parts
		if len(parts) != 2 ||
			parts[0].ToolResult.ID != "call-1" ||
			parts[1].ToolResult.ID != "call-2" {
			t.Fatalf("continuation tool result order = %#v", parts)
		}
		return runnerTextResponse("done"), nil
	}}
	// This test is about overlap, so it states the concurrency it needs: an
	// unset limit executes one call at a time.
	runner := newRunner(t, model, toolloop.Config{MaxConcurrentCalls: 2})

	type runResult struct {
		events []toolloop.Event
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		events, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
		done <- runResult{events: events, err: err}
	}()

	gotStarted := map[string]bool{}
	gotStarted[<-started] = true
	gotStarted[<-started] = true
	if !gotStarted["first"] || !gotStarted["second"] {
		t.Fatalf("started calls = %v", gotStarted)
	}

	// Complete the second call first. Observable results must still commit in
	// the model's original call order.
	close(releaseSecond)
	<-finishedSecond
	close(releaseFirst)

	result := <-done
	if result.err != nil {
		t.Fatalf("Run: %v", result.err)
	}
	var resultIDs []string
	for _, event := range result.events {
		if event.Kind == toolloop.EventToolResult {
			resultIDs = append(resultIDs, event.ToolResult.ID)
		}
	}
	if want := []string{"call-1", "call-2"}; !reflect.DeepEqual(resultIDs, want) {
		t.Fatalf("tool result event order = %v, want %v", resultIDs, want)
	}
}

func TestRunnerHonorsMaxConcurrentCalls(t *testing.T) {
	started := make(chan string, 3)
	releases := map[string]chan struct{}{
		"first":  make(chan struct{}),
		"second": make(chan struct{}),
		"third":  make(chan struct{}),
	}
	tools := []*runnerTool{}
	for _, name := range []string{"first", "second", "third"} {
		tools = append(tools, newConcurrentRunnerTool(name, "", func(context.Context, string) (string, error) {
			started <- name
			<-releases[name]
			return name, nil
		}))
	}
	registry := newRunnerRegistry(t, tools[0], tools[1], tools[2])
	model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "1", Name: "first"},
				chat.ToolCall{ID: "2", Name: "second"},
				chat.ToolCall{ID: "3", Name: "third"},
			), nil
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{MaxConcurrentCalls: 2})

	done := make(chan error, 1)
	go func() {
		_, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
		done <- err
	}()

	firstStarted := <-started
	secondStarted := <-started
	select {
	case thirdStarted := <-started:
		t.Fatalf("third call %q started before a concurrency slot was released", thirdStarted)
	default:
	}

	close(releases[firstStarted])
	thirdStarted := <-started
	close(releases[secondStarted])
	close(releases[thirdStarted])
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunnerSerializesCallsWithSameResourceKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan string, 2)
		releaseFirst := make(chan struct{})

		first := newConcurrentRunnerTool("first", "cache:tenant-1", func(context.Context, string) (string, error) {
			started <- "first"
			<-releaseFirst
			return "one", nil
		})
		second := newConcurrentRunnerTool("second", "cache:tenant-1", func(context.Context, string) (string, error) {
			started <- "second"
			return "two", nil
		})
		registry := newRunnerRegistry(t, first, second)
		model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
			if round == 1 {
				return runnerToolResponse(
					chat.ToolCall{ID: "call-1", Name: "first"},
					chat.ToolCall{ID: "call-2", Name: "second"},
				), nil
			}
			return runnerTextResponse("done"), nil
		}}
		runner := newRunner(t, model, toolloop.Config{})

		done := make(chan error, 1)
		go func() {
			_, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
			done <- err
		}()

		if got := <-started; got != "first" {
			t.Fatalf("first started call = %q", got)
		}
		synctest.Wait()
		select {
		case got := <-started:
			t.Fatalf("same-key call %q overlapped the first call", got)
		default:
		}

		close(releaseFirst)
		if got := <-started; got != "second" {
			t.Fatalf("second started call = %q", got)
		}
		if err := <-done; err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

func TestRunnerContainsConcurrentToolPanicAndKeepsSiblingResult(t *testing.T) {
	panicking := newConcurrentRunnerTool("panicking", "", func(context.Context, string) (string, error) {
		panic("boom")
	})
	sibling := newConcurrentRunnerTool("sibling", "", func(context.Context, string) (string, error) {
		return "survived", nil
	})
	registry := newRunnerRegistry(t, panicking, sibling)
	model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "call-1", Name: "panicking"},
				chat.ToolCall{ID: "call-2", Name: "sibling"},
			), nil
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})

	events, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var results []chat.ToolResult
	for _, event := range events {
		if event.Kind == toolloop.EventToolResult {
			results = append(results, *event.ToolResult)
		}
	}
	if len(results) != 2 {
		t.Fatalf("tool results = %#v", results)
	}
	if results[0].ID != "call-1" || !results[0].IsError || !strings.Contains(results[0].Result, "tool panicked: boom") {
		t.Fatalf("panic result = %#v", results[0])
	}
	if results[1].ID != "call-2" || results[1].IsError || results[1].Result != "survived" {
		t.Fatalf("sibling result = %#v", results[1])
	}
}

func TestRunnerCheckpointsMultiplePausedCallsAndCommitsInOrder(t *testing.T) {
	var (
		attemptsMu sync.Mutex
		attempts   = map[string]int{}
	)
	newApproval := func(name, pauseID string) *runnerTool {
		return newConcurrentRunnerTool(name, "", func(ctx context.Context, _ string) (string, error) {
			attemptsMu.Lock()
			attempts[name]++
			attemptsMu.Unlock()
			resume, ok := toolloop.ResumeFromContext(ctx)
			if !ok {
				return "", &toolloop.PauseError{
					ID:           pauseID,
					Reason:       name + " approval",
					Prompt:       json.RawMessage(`"` + name + `?"`),
					ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
				}
			}
			if resume.ID != pauseID {
				t.Fatalf("%s resume ID = %q, want %q", name, resume.ID, pauseID)
			}
			return name + " approved", nil
		})
	}

	first := newApproval("first", "pause-1")
	second := newApproval("second", "pause-2")
	registry := newRunnerRegistry(t, first, second)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "call-1", Name: "first"},
				chat.ToolCall{ID: "call-2", Name: "second"},
			), nil
		}
		parts := request.Messages[len(request.Messages)-1].Parts
		if len(parts) != 2 ||
			parts[0].ToolResult.Result != "first approved" ||
			parts[1].ToolResult.Result != "second approved" {
			t.Fatalf("continuation results = %#v", parts)
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})

	initial, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	firstPause := initial[len(initial)-1]
	if firstPause.Kind != toolloop.EventPause || firstPause.Pause.ID != "pause-1" {
		t.Fatalf("first pause = %#v", firstPause)
	}
	checkpoint := firstPause.Pause.Checkpoint
	if checkpoint.NextResult != 0 ||
		checkpoint.CallStates[0].Status != toolloop.CallPaused ||
		checkpoint.CallStates[1].Status != toolloop.CallPaused {
		t.Fatalf("first checkpoint = %#v", checkpoint)
	}

	afterFirst, err := collectRunnerEvents(runner.Resume(
		t.Context(),
		checkpoint,
		registry,
		toolloop.Resume{ID: "pause-1", Input: json.RawMessage(`true`)},
	))
	if err != nil {
		t.Fatalf("Resume first: %v", err)
	}
	if want := []toolloop.EventKind{
		toolloop.EventResume,
		toolloop.EventToolCall,
		toolloop.EventToolResult,
		toolloop.EventPause,
	}; !reflect.DeepEqual(eventKinds(afterFirst), want) {
		t.Fatalf("first resume events = %v, want %v", eventKinds(afterFirst), want)
	}
	secondPause := afterFirst[len(afterFirst)-1]
	if secondPause.Pause.ID != "pause-2" ||
		secondPause.Pause.Checkpoint.NextResult != 1 ||
		secondPause.Pause.Checkpoint.CallStates[0].Status != toolloop.CallCompleted ||
		secondPause.Pause.Checkpoint.CallStates[1].Status != toolloop.CallPaused {
		t.Fatalf("second pause = %#v", secondPause)
	}

	afterSecond, err := collectRunnerEvents(runner.Resume(
		t.Context(),
		secondPause.Pause.Checkpoint,
		registry,
		toolloop.Resume{ID: "pause-2", Input: json.RawMessage(`true`)},
	))
	if err != nil {
		t.Fatalf("Resume second: %v", err)
	}
	attemptsMu.Lock()
	firstAttempts, secondAttempts := attempts["first"], attempts["second"]
	attemptsMu.Unlock()
	if firstAttempts != 2 || secondAttempts != 2 {
		t.Fatalf("attempts = first:%d second:%d, want two per tool", firstAttempts, secondAttempts)
	}
	if want := []toolloop.EventKind{
		toolloop.EventResume,
		toolloop.EventToolCall,
		toolloop.EventToolResult,
		toolloop.EventModelRequest,
		toolloop.EventModelResponse,
	}; !reflect.DeepEqual(eventKinds(afterSecond), want) {
		t.Fatalf("second resume events = %v, want %v", eventKinds(afterSecond), want)
	}
}

func TestRunnerContinuesHostCompletedPausedCallInOrder(t *testing.T) {
	newApproval := func(name, pauseID string) *runnerTool {
		return newConcurrentRunnerTool(name, "", func(ctx context.Context, _ string) (string, error) {
			if _, ok := toolloop.ResumeFromContext(ctx); ok {
				return name + " approved", nil
			}
			return "", &toolloop.PauseError{
				ID:           pauseID,
				Reason:       name + " approval",
				Prompt:       json.RawMessage(`"` + name + `?"`),
				ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
			}
		})
	}
	first := newApproval("first", "pause-1")
	second := newApproval("second", "pause-2")
	registry := newRunnerRegistry(t, first, second)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "call-1", Name: "first"},
				chat.ToolCall{ID: "call-2", Name: "second"},
			), nil
		}
		parts := request.Messages[len(request.Messages)-1].Parts
		if len(parts) != 2 ||
			parts[0].ToolResult.Result != "error: delegated child canceled" ||
			parts[1].ToolResult.Result != "second approved" {
			t.Fatalf("continuation results = %#v", parts)
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})

	initial, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	checkpoint := initial[len(initial)-1].Pause.Checkpoint
	completed, err := checkpoint.CompletePausedCall("call-1", chat.ToolResult{
		ID:      "call-1",
		Name:    "first",
		Result:  "error: delegated child canceled",
		IsError: true,
	})
	if err != nil {
		t.Fatalf("CompletePausedCall: %v", err)
	}
	if _, awaiting, err := completed.AwaitingInput(); err != nil || awaiting {
		t.Fatalf("AwaitingInput = %v, %v; want false, nil", awaiting, err)
	}
	if checkpoint.CallStates[0].Status != toolloop.CallPaused {
		t.Fatal("CompletePausedCall mutated its receiver")
	}

	continued, err := collectRunnerEvents(runner.Continue(t.Context(), completed, registry))
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if want := []toolloop.EventKind{toolloop.EventToolResult, toolloop.EventPause}; !reflect.DeepEqual(eventKinds(continued), want) {
		t.Fatalf("continue events = %v, want %v", eventKinds(continued), want)
	}
	next := continued[len(continued)-1].Pause.Checkpoint
	if next.NextResult != 1 || next.ID != "pause-2" {
		t.Fatalf("next checkpoint = %#v", next)
	}

	finished, err := collectRunnerEvents(runner.Resume(
		t.Context(),
		next,
		registry,
		toolloop.Resume{ID: "pause-2", Input: json.RawMessage(`true`)},
	))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if len(finished) == 0 || !finished[len(finished)-1].Final {
		t.Fatalf("finished events = %#v", finished)
	}
}

func TestRunnerContinuesInternallyReadyPausedToolWithoutResumeEvent(t *testing.T) {
	calls := 0
	tool := newRunnerTool("delegated", func(ctx context.Context, _ string) (string, error) {
		calls++
		if _, resumed := toolloop.ResumeFromContext(ctx); resumed {
			t.Fatal("ContinuePaused attached external resume input")
		}
		if calls == 1 {
			return "", &toolloop.PauseError{
				ID:           "child-wait",
				Reason:       "delegated child waiting",
				Prompt:       json.RawMessage(`"wait?"`),
				ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
			}
		}
		return "child completed internally", nil
	})
	tool.inputless = true
	registry := newRunnerRegistry(t, tool)
	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(chat.ToolCall{ID: "call-1", Name: "delegated"}), nil
		}
		result := request.Messages[len(request.Messages)-1].Parts[0].ToolResult
		if result == nil || result.Result != "child completed internally" {
			t.Fatalf("continuation result = %#v", result)
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})

	initial, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	checkpoint := initial[len(initial)-1].Pause.Checkpoint
	continued, err := collectRunnerEvents(runner.ContinuePaused(t.Context(), checkpoint, registry))
	if err != nil {
		t.Fatalf("ContinuePaused: %v", err)
	}
	if want := []toolloop.EventKind{
		toolloop.EventToolCall,
		toolloop.EventToolResult,
		toolloop.EventModelRequest,
		toolloop.EventModelResponse,
	}; !reflect.DeepEqual(eventKinds(continued), want) {
		t.Fatalf("ContinuePaused events = %v, want %v", eventKinds(continued), want)
	}
	for _, event := range continued {
		if event.Kind == toolloop.EventResume {
			t.Fatal("ContinuePaused emitted a Resume event")
		}
	}
	if calls != 2 {
		t.Fatalf("tool calls = %d, want initial pause + internal continuation", calls)
	}
}

func TestRunnerRejectsInputlessContinuationWithoutToolCapability(t *testing.T) {
	calls := 0
	tool := newRunnerTool("approval", func(context.Context, string) (string, error) {
		calls++
		return "", &toolloop.PauseError{
			ID:           "approval-wait",
			Reason:       "approval required",
			Prompt:       json.RawMessage(`"approve?"`),
			ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
		}
	})
	registry := newRunnerRegistry(t, tool)
	model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
		return runnerToolResponse(chat.ToolCall{ID: "call-1", Name: "approval"}), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	initial, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := initial[len(initial)-1].Pause.Checkpoint

	if _, err := collectRunnerEvents(runner.ContinuePaused(t.Context(), checkpoint, registry)); !errors.Is(err, toolloop.ErrInvalidInput) {
		t.Fatalf("ContinuePaused error = %v, want ErrInvalidInput", err)
	}
	if calls != 1 {
		t.Fatalf("tool calls = %d, want no re-entry without capability", calls)
	}
}

func TestCheckpointCompletesNonActivePausedCallWithoutMovingBoundary(t *testing.T) {
	first := newConcurrentRunnerTool("first", "", func(context.Context, string) (string, error) {
		return "", &toolloop.PauseError{
			ID: "pause-1", Reason: "first approval",
			Prompt: json.RawMessage(`"first?"`), ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
		}
	})
	second := newConcurrentRunnerTool("second", "", func(context.Context, string) (string, error) {
		return "", &toolloop.PauseError{
			ID: "pause-2", Reason: "second approval",
			Prompt: json.RawMessage(`"second?"`), ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
		}
	})
	registry := newRunnerRegistry(t, first, second)
	model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
		return runnerToolResponse(
			chat.ToolCall{ID: "call-1", Name: "first"},
			chat.ToolCall{ID: "call-2", Name: "second"},
		), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	events, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	checkpoint := events[len(events)-1].Pause.Checkpoint

	completed, err := checkpoint.CompletePausedCall("call-2", chat.ToolResult{
		ID: "call-2", Name: "second", Result: "error: delegated child canceled", IsError: true,
	})
	if err != nil {
		t.Fatalf("CompletePausedCall: %v", err)
	}
	pending, awaiting, err := completed.AwaitingInput()
	if err != nil || !awaiting || pending.ID != "pause-1" {
		t.Fatalf("AwaitingInput = %#v, %v, %v", pending, awaiting, err)
	}
	if completed.NextResult != 0 ||
		completed.CallStates[0].Status != toolloop.CallPaused ||
		completed.CallStates[1].Status != toolloop.CallCompleted {
		t.Fatalf("completed checkpoint = %#v", completed)
	}
}

func TestRunnerBuffersLaterCompletedResultBehindEarlierPause(t *testing.T) {
	first := newConcurrentRunnerTool("first", "", func(ctx context.Context, _ string) (string, error) {
		if _, ok := toolloop.ResumeFromContext(ctx); !ok {
			return "", &toolloop.PauseError{
				ID:           "pause-1",
				Reason:       "approval",
				Prompt:       json.RawMessage(`"approve?"`),
				ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
			}
		}
		return "first", nil
	})
	second := newConcurrentRunnerTool("second", "", func(context.Context, string) (string, error) {
		return "second", nil
	})
	registry := newRunnerRegistry(t, first, second)
	model := &scriptedModel{call: func(round int, _ *chat.Request) (*chat.Response, error) {
		if round == 1 {
			return runnerToolResponse(
				chat.ToolCall{ID: "call-1", Name: "first"},
				chat.ToolCall{ID: "call-2", Name: "second"},
			), nil
		}
		return runnerTextResponse("done"), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})

	initial, err := collectRunnerEvents(runner.Run(t.Context(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, event := range initial {
		if event.Kind == toolloop.EventToolResult {
			t.Fatalf("later result was published before earlier pause: %#v", event)
		}
	}
	checkpoint := initial[len(initial)-1].Pause.Checkpoint
	if checkpoint.CallStates[0].Status != toolloop.CallPaused ||
		checkpoint.CallStates[1].Status != toolloop.CallCompleted {
		t.Fatalf("checkpoint states = %#v", checkpoint.CallStates)
	}

	resumed, err := collectRunnerEvents(runner.Resume(
		t.Context(),
		checkpoint,
		registry,
		toolloop.Resume{ID: "pause-1", Input: json.RawMessage(`true`)},
	))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var ids []string
	for _, event := range resumed {
		if event.Kind == toolloop.EventToolResult {
			ids = append(ids, event.ToolResult.ID)
		}
	}
	if want := []string{"call-1", "call-2"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("resumed result order = %v, want %v", ids, want)
	}
}
