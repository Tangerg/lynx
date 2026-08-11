package runtimeembedded

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type runBindingStub struct {
	start     func(context.Context, protocol.StartRunRequest, embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	resume    func(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	subscribe func(context.Context, protocol.SubscribeRunRequest, embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	cancel    func(context.Context, protocol.CancelRunRequest, embedded.CommandOptions) (*protocol.CancelRunResponse, error)
	steer     func(context.Context, protocol.SteerRunRequest, embedded.CommandOptions) error
}

func (s runBindingStub) SteerRun(ctx context.Context, request protocol.SteerRunRequest, options embedded.CommandOptions) error {
	if s.steer == nil {
		return errors.New("unexpected steer")
	}
	return s.steer(ctx, request, options)
}

func (s runBindingStub) StartRun(ctx context.Context, request protocol.StartRunRequest, options embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return s.start(ctx, request, options)
}

func (s runBindingStub) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest, options embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return s.resume(ctx, request, options)
}

func (s runBindingStub) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest, options embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return s.subscribe(ctx, request, options)
}

func (s runBindingStub) CancelRun(ctx context.Context, request protocol.CancelRunRequest, options embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
	return s.cancel(ctx, request, options)
}

func TestStartRunMapsOptionsAndProjectsAtomicStream(t *testing.T) {
	const (
		runID     = "run_1"
		segmentID = "seg_1"
	)
	stub := runBindingStub{}
	stub.start = func(_ context.Context, request protocol.StartRunRequest, options embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if request.SessionID != "ses_1" || len(request.Input) != 1 || request.Input[0].Text != "hello" {
			t.Fatalf("start request = %+v", request)
		}
		if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion ||
			options.RequestMeta.ClientInfo == nil || options.RequestMeta.ClientInfo.Name != clientName {
			t.Fatalf("start options = %+v", options)
		}
		return &protocol.StartRunResponse{RunID: runID, SegmentID: segmentID, UserItemID: "item_user"}, func(yield func(protocol.RunEvent, error) bool) {
			yield(protocol.RunEvent{
				RunID: runID, SegmentID: segmentID, EventID: "evt_1", Timestamp: time.Unix(1, 0),
				Event: protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &protocol.RunRef{
					RunSummary:      protocol.RunSummary{ID: runID, SessionID: "ses_1", Status: protocol.RunStatusRunning},
					ActiveSegmentID: segmentID,
					ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
				}},
			}, nil)
			yield(protocol.RunEvent{
				RunID: runID, SegmentID: segmentID, EventID: "evt_2", Timestamp: time.Unix(2, 0),
				Event: protocol.StreamEvent{
					Type:    protocol.StreamSegmentFinished,
					Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentCompleted},
					Metrics: &protocol.RunMetrics{},
				},
			}, nil)
		}, nil
	}
	runtime := &Runtime{runs: stub, meta: requestMeta("test"), loadAttachment: loadAttachmentFile}
	stream, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_1", Message: agent.Message{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if stream.RunID != runID || stream.SegmentID != segmentID || stream.UserItemID != "item_user" {
		t.Fatalf("stream = %+v", stream)
	}
	var events []agent.RunEvent
	for event, err := range stream.Events {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		events = append(events, event)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if _, ok := events[0].Event.(agent.SegmentStarted); !ok {
		t.Fatalf("first event = %T", events[0].Event)
	}
	if finished, ok := events[1].Event.(agent.RunFinished); !ok || finished.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("second event = %+v", events[1].Event)
	}
}

func TestSubscribeRunPassesOpaqueReplayCursor(t *testing.T) {
	stub := runBindingStub{}
	stub.subscribe = func(_ context.Context, request protocol.SubscribeRunRequest, options embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if request.RunID != "run_1" || request.SegmentID != "seg_1" || options.AfterEventID != "opaque:event/cursor" {
			t.Fatalf("subscribe = (%+v, %+v)", request, options)
		}
		return &protocol.SubscribeRunResponse{RunID: request.RunID, SegmentID: request.SegmentID, HeadEventID: "head"}, func(func(protocol.RunEvent, error) bool) {}, nil
	}
	runtime := &Runtime{runs: stub, meta: requestMeta("test")}
	stream, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{
		RunID: "run_1", SegmentID: "seg_1", AfterEventID: "opaque:event/cursor",
	})
	if err != nil {
		t.Fatalf("SubscribeRun: %v", err)
	}
	if stream.HeadEventID != "head" {
		t.Fatalf("head = %q", stream.HeadEventID)
	}
}

func TestProjectedStreamClassifiesClosedRuntime(t *testing.T) {
	stream := projectEventStream(func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{}, embedded.ErrClosed)
	})
	for _, err := range stream {
		if !errors.Is(err, agent.ErrDisconnected) || !errors.Is(err, embedded.ErrClosed) {
			t.Fatalf("stream error = %v", err)
		}
		return
	}
	t.Fatal("stream yielded no error")
}

func TestResumeAndCancelMapControlContracts(t *testing.T) {
	stub := runBindingStub{}
	stub.resume = func(_ context.Context, request protocol.ResumeRunRequest, options embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if options.IdempotencyKey == "" || request.RunID != "run_1" || len(request.Responses) != 1 {
			t.Fatalf("resume = (%+v, %+v)", request, options)
		}
		answer := request.Responses[0]
		if answer.ItemID != "item_approval" || answer.Response.Type != protocol.InterruptResponseApproval ||
			answer.Response.Decision != protocol.ApprovalApprove || answer.Response.Remember == nil ||
			answer.Response.Remember.Scope != protocol.RememberProject {
			t.Fatalf("resume answer = %+v", answer)
		}
		return &protocol.ResumeRunResponse{RunID: request.RunID, SegmentID: "seg_2"}, func(func(protocol.RunEvent, error) bool) {}, nil
	}
	stub.cancel = func(_ context.Context, request protocol.CancelRunRequest, options embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
		if request.RunID != "run_1" || request.Reason != "stop" || options.IdempotencyKey == "" {
			t.Fatalf("cancel = (%+v, %+v)", request, options)
		}
		return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: protocol.RunRef{
			RunSummary: protocol.RunSummary{
				ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
				Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Detail: "stop"},
			},
			ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
		}}, nil
	}
	runtime := &Runtime{runs: stub, meta: requestMeta("test")}
	resumed, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: "run_1", Answers: []agent.InterruptAnswer{{
			ItemID: "item_approval",
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberProject},
		}},
	})
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if resumed.RunID != "run_1" || resumed.SegmentID != "seg_2" {
		t.Fatalf("resumed = %+v", resumed)
	}
	canceled, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: "run_1", Reason: "stop"})
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if canceled.Status != agent.RunStatusFinished || canceled.Outcome.Status != agent.OutcomeCanceled || canceled.Outcome.Detail != "stop" {
		t.Fatalf("canceled = %+v", canceled)
	}
}

func TestSteerRunBindsStructuredInputToTheObservedSegment(t *testing.T) {
	stub := runBindingStub{}
	stub.steer = func(_ context.Context, request protocol.SteerRunRequest, options embedded.CommandOptions) error {
		if request.RunID != "run_1" || request.ExpectedSegmentID != "seg_2" || len(request.Input) != 1 || request.Input[0].Text != "focus on the parser" {
			t.Fatalf("steer request = %+v", request)
		}
		if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
			t.Fatalf("steer options = %+v", options)
		}
		return protocol.ErrStaleSegment
	}
	runtime := &Runtime{runs: stub, meta: requestMeta("test"), loadAttachment: loadAttachmentFile}
	err := runtime.SteerRun(t.Context(), agent.SteerRun{
		RunID: "run_1", SegmentID: "seg_2", Message: agent.Message{Text: "focus on the parser"},
	})
	if !errors.Is(err, agent.ErrStaleSegment) {
		t.Fatalf("SteerRun error = %v, want ErrStaleSegment", err)
	}
}
