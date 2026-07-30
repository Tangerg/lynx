package toolloop

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// runnerState is one run's mutable position in the loop, owned by the single
// goroutine driving it. Concurrent tool calls never reach it: each writes its
// own slot in a pre-sized outcome slice, and the driver folds those in only
// after the group has joined. Nothing here takes a lock, so a future path that
// mutates it from a tool goroutine would race silently.
type runnerState struct {
	request    *chat.Request
	resolver   ToolResolver
	round      int
	response   *chat.Response
	calls      []chat.ToolCall
	callStates []CallCheckpoint
	nextResult int
	resume     *Resume
	// continuePaused re-enters a paused tool whose durable dependency was
	// settled outside the tool loop. Unlike resume, it carries no user input
	// and emits no Resume event.
	continuePaused bool
	// promotions collects tools promoted mid-loop (see PromoteTools). It is
	// drained into request.Tools before every checkpoint or continuation, so a
	// promoted tool is advertised on the next model round and rides through a
	// pause/resume inside the checkpoint's request.
	promotions toolPromotions
}

func (s *runnerState) validateInput() error {
	if s == nil || s.request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidInput)
	}
	if err := s.request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidInput, err)
	}
	if len(s.request.Tools) == 0 {
		return nil
	}
	if valueIsNil(s.resolver) {
		return fmt.Errorf("%w: request advertises tools but resolver is nil", ErrInvalidInput)
	}
	for _, definition := range s.request.Tools {
		hosted, matched, err := executableFor(s.resolver, definition)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		if hosted.tool == nil {
			return fmt.Errorf("%w: advertised tool %q is not executable", ErrInvalidInput, definition.Name)
		}
		if !matched {
			return fmt.Errorf("%w: advertised tool %q definition does not match executable tool", ErrInvalidInput, definition.Name)
		}
	}
	return nil
}

func (s *runnerState) startedCalls() int {
	for index, state := range s.callStates {
		if state.Status == CallQueued {
			return index
		}
	}
	return len(s.callStates)
}

func (s *runnerState) settled(index int, result chat.ToolResult, pending *PendingCall) {
	if pending != nil {
		cloned := *pending
		s.callStates[index] = CallCheckpoint{Status: CallPaused, Pending: &cloned}
		return
	}
	cloned := result
	s.callStates[index] = CallCheckpoint{Status: CallCompleted, Result: &cloned}
}

func (s *runnerState) continuationRequest() (*chat.Request, error) {
	choice := s.response.First()
	if choice == nil || choice.Message == nil {
		return nil, errors.New("toolloop: tool response has no canonical assistant message")
	}
	continuation, err := snapshot(s.request)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot continuation request: %w", err)
	}
	assistant, err := snapshot(choice.Message)
	if err != nil {
		return nil, fmt.Errorf("toolloop: snapshot assistant message: %w", err)
	}
	results, err := s.completedResults()
	if err != nil {
		return nil, err
	}
	continuation.Messages = append(continuation.Messages, *assistant, chat.NewToolMessage(results...))
	if err := continuation.Validate(); err != nil {
		return nil, fmt.Errorf("toolloop: continuation request: %w", err)
	}
	return continuation, nil
}

func (s *runnerState) completedResults() ([]chat.ToolResult, error) {
	if len(s.callStates) != len(s.calls) {
		return nil, errors.New("toolloop: call state count does not match response calls")
	}
	results := make([]chat.ToolResult, len(s.callStates))
	for index, state := range s.callStates {
		if state.Status != CallCompleted || state.Result == nil {
			return nil, fmt.Errorf("toolloop: call %q has no completed result", s.calls[index].ID)
		}
		results[index] = *state.Result
	}
	return results, nil
}
