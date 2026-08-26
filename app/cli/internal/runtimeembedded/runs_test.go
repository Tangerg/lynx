package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type runBindingStub struct {
	start     func(context.Context, protocol.StartRunRequest, embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	resume    func(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	subscribe func(context.Context, protocol.SubscribeRunRequest, embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	cancel    func(context.Context, protocol.CancelRunRequest, embedded.CommandOptions) (*protocol.CancelRunResponse, error)
	steer     func(context.Context, protocol.SteerRunRequest, embedded.CommandOptions) error
}

func (r runBindingStub) SteerRun(ctx context.Context, request protocol.SteerRunRequest, options embedded.CommandOptions) error {
	if r.steer == nil {
		return errors.New("unexpected steer")
	}
	return r.steer(ctx, request, options)
}

func (r runBindingStub) StartRun(ctx context.Context, request protocol.StartRunRequest, options embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.start(ctx, request, options)
}

func (r runBindingStub) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest, options embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.resume(ctx, request, options)
}

func (r runBindingStub) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest, options embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.subscribe(ctx, request, options)
}

func (r runBindingStub) CancelRun(ctx context.Context, request protocol.CancelRunRequest, options embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
	return r.cancel(ctx, request, options)
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

func TestRunMutationsPreserveCallerCommandIdentity(t *testing.T) {
	commandID := agent.CommandID("cli_0123456789abcdef0123456789abcdef")
	const namespace = "idp_test"
	stub := runBindingStub{}
	stub.start = func(_ context.Context, request protocol.StartRunRequest, options embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if options.IdempotencyKey != string(commandID) || options.IdempotencyNamespace != namespace {
			t.Fatalf("start idempotency options = %+v", options)
		}
		return &protocol.StartRunResponse{RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1"}, func(func(protocol.RunEvent, error) bool) {}, nil
	}
	stub.resume = func(_ context.Context, request protocol.ResumeRunRequest, options embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if options.IdempotencyKey != string(commandID) || options.IdempotencyNamespace != namespace {
			t.Fatalf("resume idempotency options = %+v", options)
		}
		return &protocol.ResumeRunResponse{RunID: request.RunID, SegmentID: "seg_2"}, func(func(protocol.RunEvent, error) bool) {}, nil
	}
	stub.cancel = func(_ context.Context, request protocol.CancelRunRequest, options embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
		if options.IdempotencyKey != string(commandID) || options.IdempotencyNamespace != namespace {
			t.Fatalf("cancel idempotency options = %+v", options)
		}
		return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: protocol.RunRef{
			RunSummary:      protocol.RunSummary{ID: request.RunID, SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCanceled}},
			ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
		}}, nil
	}
	stub.steer = func(_ context.Context, _ protocol.SteerRunRequest, options embedded.CommandOptions) error {
		if options.IdempotencyKey != string(commandID) || options.IdempotencyNamespace != namespace {
			t.Fatalf("steer idempotency options = %+v", options)
		}
		return nil
	}
	runtime := &Runtime{
		runs: stub, meta: requestMeta("test"),
		profile: runtimeprofile.Profile{Limits: runtimeprofile.Limits{IdempotencyNamespace: namespace}},
	}
	if _, err := runtime.StartRun(t.Context(), agent.StartRun{CommandID: commandID, SessionID: "ses_1", Message: agent.Message{Text: "start"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{CommandID: commandID, RunID: "run_1", Answers: []agent.InterruptAnswer{{ItemID: "item_approval", Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{CommandID: commandID, RunID: "run_1"}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SteerRun(t.Context(), agent.SteerRun{CommandID: commandID, RunID: "run_1", SegmentID: "seg_1", Message: agent.Message{Text: "steer"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRunInputMutationsRejectImagesBeforeCallingBindingWithoutMultimodalCapability(t *testing.T) {
	t.Parallel()
	attachment := agent.Attachment{
		ID: "image", Kind: agent.AttachmentImage, Name: "image.png", Path: "/image.png",
		MimeType: "image/png", Size: 5,
	}
	message := agent.Message{Attachments: []agent.Attachment{attachment}}
	for _, test := range []struct {
		name string
		call func(context.Context, *Runtime) error
	}{
		{
			name: "start",
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.StartRun(ctx, agent.StartRun{SessionID: "ses_1", Message: message})
				return err
			},
		},
		{
			name: "resume",
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.ResumeRun(ctx, agent.ResumeRun{
					RunID: "run_1", Message: &message,
					Answers: []agent.InterruptAnswer{{
						ItemID: "item_approval", Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
					}},
				})
				return err
			},
		},
		{
			name: "steer",
			call: func(ctx context.Context, runtime *Runtime) error {
				return runtime.SteerRun(ctx, agent.SteerRun{RunID: "run_1", SegmentID: "seg_1", Message: message})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			called := false
			stub := runBindingStub{
				start: func(context.Context, protocol.StartRunRequest, embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
					called = true
					return nil, nil, nil
				},
				resume: func(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
					called = true
					return nil, nil, nil
				},
				steer: func(context.Context, protocol.SteerRunRequest, embedded.CommandOptions) error {
					called = true
					return nil
				},
			}
			runtime := &Runtime{
				runs: stub,
				loadAttachment: func(context.Context, string, int64) ([]byte, error) {
					t.Fatal("image was read without multimodal capability")
					return nil, nil
				},
			}
			err := test.call(t.Context(), runtime)
			if err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
				t.Fatalf("run mutation error = %v, want ErrIncompatibleRuntime", err)
			}
			if called {
				t.Fatal("run mutation reached binding without multimodal capability")
			}
		})
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
	}, "seg_1")
	for _, err := range stream {
		if !errors.Is(err, agent.ErrDisconnected) || !errors.Is(err, embedded.ErrClosed) {
			t.Fatalf("stream error = %v", err)
		}
		return
	}
	t.Fatal("stream yielded no error")
}

func TestProjectedStreamClassifiesMalformedRuntimeEvents(t *testing.T) {
	t.Parallel()
	stream := projectEventStream(func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{}, nil)
	}, "seg_1")
	for _, err := range stream {
		requireRuntimeContractViolation(t, err)
		return
	}
	t.Fatal("malformed stream yielded no error")
}

func TestRunAdaptersRejectMismatchedAcknowledgements(t *testing.T) {
	t.Parallel()
	events := func(func(protocol.RunEvent, error) bool) {}
	runtime := &Runtime{runs: runBindingStub{
		resume: func(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
			return &protocol.ResumeRunResponse{RunID: "run_other", SegmentID: "seg_2"}, events, nil
		},
		subscribe: func(context.Context, protocol.SubscribeRunRequest, embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
			return &protocol.SubscribeRunResponse{RunID: "run_1", SegmentID: "seg_other"}, events, nil
		},
	}, meta: requestMeta("test")}

	stream, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: "run_1", Answers: []agent.InterruptAnswer{{
		ItemID: "approval", Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
	}}})
	requireRuntimeContractViolation(t, err)
	receipt, accepted := agent.AcceptedMutationReceipt(err)
	if !accepted || !segmentStreamEmpty(stream) || receipt.RunID != "run_other" || receipt.SegmentID != "seg_2" {
		t.Fatalf("accepted mismatched resume = stream %+v, receipt %+v, accepted %t", stream, receipt, accepted)
	}
	_, err = runtime.SubscribeRun(t.Context(), agent.SubscribeRun{RunID: "run_1", SegmentID: "seg_1"})
	requireRuntimeContractViolation(t, err)
}

func TestRunMutationAdaptersPreservePartialAcceptedReceipts(t *testing.T) {
	t.Parallel()
	events := func(func(protocol.RunEvent, error) bool) {}
	runtime := &Runtime{runs: runBindingStub{
		start: func(context.Context, protocol.StartRunRequest, embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
			return &protocol.StartRunResponse{RunID: "run_started", SegmentID: "seg_started"}, events, nil
		},
		resume: func(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
			return &protocol.ResumeRunResponse{RunID: "run_1", SegmentID: ""}, events, nil
		},
	}, meta: requestMeta("test")}

	started, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: "ses_1", Message: agent.Message{Text: "start"}})
	requireRuntimeContractViolation(t, err)
	receipt, accepted := agent.AcceptedMutationReceipt(err)
	if !accepted || !segmentStreamEmpty(started) || receipt.RunID != "run_started" || receipt.SegmentID != "seg_started" {
		t.Fatalf("partial accepted start = stream %+v, receipt %+v, accepted %t", started, receipt, accepted)
	}

	resumed, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: "run_1", Answers: []agent.InterruptAnswer{{
		ItemID: "approval", Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
	}}})
	requireRuntimeContractViolation(t, err)
	receipt, accepted = agent.AcceptedMutationReceipt(err)
	if !accepted || !segmentStreamEmpty(resumed) || receipt.RunID != "run_1" || receipt.SegmentID != "" {
		t.Fatalf("partial accepted resume = stream %+v, receipt %+v, accepted %t", resumed, receipt, accepted)
	}
}

func segmentStreamEmpty(stream agent.SegmentStream) bool {
	return stream.RunID == "" && stream.SegmentID == "" && stream.UserItemID == "" &&
		stream.HeadEventID == "" && stream.Events == nil
}

func TestResumeAndCancelMapControlContracts(t *testing.T) {
	override, err := agent.ParseToolArgumentOverride([]byte(`{"command":"go test -race ./...","count":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	stub := runBindingStub{}
	stub.resume = func(_ context.Context, request protocol.ResumeRunRequest, options embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if options.IdempotencyKey == "" || request.RunID != "run_1" || len(request.Responses) != 1 {
			t.Fatalf("resume = (%+v, %+v)", request, options)
		}
		answer := request.Responses[0]
		if answer.ItemID != "item_approval" || answer.Response.Type != protocol.InterruptResponseApproval ||
			answer.Response.Decision != protocol.ApprovalApprove || answer.Response.Remember == nil ||
			answer.Response.Remember.Scope != protocol.RememberProject ||
			answer.Response.EditedArgs["command"] != "go test -race ./..." ||
			fmt.Sprint(answer.Response.EditedArgs["count"]) != "9007199254740993" {
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
			Answer: agent.ApprovalAnswer{
				Decision: agent.ApprovalApprove, Remember: agent.RememberProject, ArgumentOverride: override,
			},
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
	if !canceled.Canceled.Equal(canceled.Root) || canceled.Canceled.Status != agent.RunStatusFinished ||
		canceled.Canceled.Outcome.Status != agent.OutcomeCanceled || canceled.Canceled.Outcome.Detail != "stop" {
		t.Fatalf("canceled = %+v", canceled)
	}
}

func TestProjectAnswerMapsRememberedDenial(t *testing.T) {
	t.Parallel()
	projected, err := projectAnswer(agent.InterruptAnswer{
		ItemID: "item_denial",
		Answer: agent.ApprovalAnswer{
			Decision: agent.ApprovalDeny, Remember: agent.RememberGlobal, Reason: "protect generated files",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projected.Response.ValidateWire(); err != nil {
		t.Fatalf("projected denial violates runtime wire contract: %v", err)
	}
	if projected.Response.Type != protocol.InterruptResponseApproval ||
		projected.Response.Decision != protocol.ApprovalDeny || projected.Response.Remember == nil ||
		projected.Response.Remember.Scope != protocol.RememberGlobal ||
		projected.Response.Reason != "protect generated files" || projected.Response.EditedArgs != nil {
		t.Fatalf("projected denial = %+v", projected)
	}
}

func TestProjectAnswerPreservesQuestionFieldOrderAndOwnsValues(t *testing.T) {
	t.Parallel()
	answer := agent.QuestionAnswer{Values: [][]string{
		{"concise explanation"},
		{"linux", "darwin"},
		{"custom target"},
	}}
	projected, err := projectAnswer(agent.InterruptAnswer{ItemID: "item_question", Answer: answer})
	if err != nil {
		t.Fatal(err)
	}
	if err := projected.Response.ValidateWire(); err != nil {
		t.Fatalf("projected question violates runtime wire contract: %v", err)
	}
	if projected.Response.Type != protocol.InterruptResponseAnswer ||
		!slices.EqualFunc(projected.Response.Answers, answer.Values, slices.Equal) {
		t.Fatalf("projected question = %+v", projected)
	}
	projected.Response.Answers[0][0] = "mutated"
	if answer.Values[0][0] != "concise explanation" {
		t.Fatal("projected question shared nested answer storage with the caller")
	}
}

func TestCancelRunProjectsChildAndSurvivingRootAtomically(t *testing.T) {
	stub := runBindingStub{}
	stub.cancel = func(_ context.Context, request protocol.CancelRunRequest, _ embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
		if request.RunID != "run_child" {
			t.Fatalf("request = %+v", request)
		}
		root := protocol.RunRef{
			RunSummary: protocol.RunSummary{ID: "run_root", SessionID: "ses_1", Status: protocol.RunStatusWaiting},
			ProtocolProfile: protocol.RunProtocolProfile{
				RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
				InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval},
			},
		}
		return &protocol.CancelRunResponse{
			Type: protocol.CancelRunChild,
			Run: protocol.RunRef{
				RunSummary: protocol.RunSummary{
					ID: "run_child", SessionID: "ses_1", SpawnedByItemID: "item_spawn",
					ParentRunID: "run_root", RootRunID: "run_root", Status: protocol.RunStatusFinished,
					Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Detail: "stop child"},
				},
				ProtocolProfile: root.ProtocolProfile,
			},
			RootRun: &root,
		}, nil
	}
	runtime := &Runtime{runs: stub, meta: requestMeta("test")}
	result, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: "run_child", Reason: "stop child"})
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if result.Canceled.ID != "run_child" || result.Canceled.Lineage.RootRunID != "run_root" ||
		result.Root.ID != "run_root" || result.Root.Status != agent.RunStatusWaiting {
		t.Fatalf("result = %+v", result)
	}
}

func TestCancelRunRejectsMalformedClosedResults(t *testing.T) {
	t.Parallel()
	canceled := protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
			Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCanceled},
		},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
	}
	root := canceled
	for _, test := range []struct {
		name     string
		response *protocol.CancelRunResponse
	}{
		{name: "unknown type", response: &protocol.CancelRunResponse{Type: "future", Run: canceled}},
		{name: "root carries duplicate", response: &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: canceled, RootRun: &root}},
		{name: "child omits root", response: &protocol.CancelRunResponse{Type: protocol.CancelRunChild, Run: canceled}},
		{name: "wrong addressed run", response: &protocol.CancelRunResponse{
			Type: protocol.CancelRunRoot,
			Run: func() protocol.RunRef {
				value := canceled
				value.ID = "run_other"
				return value
			}(),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := runBindingStub{cancel: func(context.Context, protocol.CancelRunRequest, embedded.CommandOptions) (*protocol.CancelRunResponse, error) {
				return test.response, nil
			}}
			runtime := &Runtime{runs: stub, meta: requestMeta("test")}
			if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: "run_1"}); !errors.Is(err, agent.ErrIncompatibleRuntime) {
				t.Fatalf("CancelRun error = %v, want ErrIncompatibleRuntime", err)
			}
		})
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
