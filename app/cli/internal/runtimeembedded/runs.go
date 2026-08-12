package runtimeembedded

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type runBinding interface {
	StartRun(context.Context, protocol.StartRunRequest, embedded.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	ResumeRun(context.Context, protocol.ResumeRunRequest, embedded.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	SubscribeRun(context.Context, protocol.SubscribeRunRequest, embedded.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	SteerRun(context.Context, protocol.SteerRunRequest, embedded.CommandOptions) error
	CancelRun(context.Context, protocol.CancelRunRequest, embedded.CommandOptions) (*protocol.CancelRunResponse, error)
}

func (r *Runtime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	if err := input.Validate(); err != nil {
		return agent.SegmentStream{}, err
	}
	content, err := r.projectInput(ctx, input.Message)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	options, err := r.runCommandOptionsFor(input.CommandID)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	request := protocol.StartRunRequest{
		SessionID: input.SessionID, Input: content,
		Provider: input.Options.Provider, Model: input.Options.Model,
		MaxTotalTokens: input.Options.Limits.MaxTotalTokens,
		MaxSteps:       input.Options.Limits.MaxSteps,
		MaxBudgetUSD:   input.Options.Limits.MaxBudgetUSD,
	}
	if generationParamsPresent(input.Options.Generation) {
		request.Params = &protocol.GenerationParams{
			Temperature: input.Options.Generation.Temperature,
			MaxTokens:   input.Options.Generation.MaxTokens,
			TopP:        input.Options.Generation.TopP,
			Stop:        slices.Clone(input.Options.Generation.Stop),
		}
	}
	ack, events, err := r.runs.StartRun(ctx, request, options)
	if err != nil {
		return agent.SegmentStream{}, classifyError(err)
	}
	stream := agent.SegmentStream{}
	if ack != nil {
		stream.RunID, stream.SegmentID, stream.UserItemID = ack.RunID, ack.SegmentID, ack.UserItemID
	}
	if events != nil {
		stream.Events = projectEventStream(events, stream.SegmentID)
	}
	if ack == nil || events == nil {
		return agent.SegmentStream{}, agent.NewAcceptedMutationError(
			stream, runtimeContractViolation("start run returned an incomplete stream"),
		)
	}
	if err := stream.ValidateStart(); err != nil {
		return agent.SegmentStream{}, agent.NewAcceptedMutationError(
			stream, runtimeContractViolation("start run returned an invalid stream: %v", err),
		)
	}
	return stream, nil
}

func generationParamsPresent(value agent.GenerationParams) bool {
	return value.Temperature != nil || value.MaxTokens != nil || value.TopP != nil || len(value.Stop) != 0
}

func (r *Runtime) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	if err := input.Validate(); err != nil {
		return agent.SegmentStream{}, err
	}
	request := protocol.ResumeRunRequest{RunID: input.RunID, Responses: make([]protocol.InterruptResponse, 0, len(input.Answers))}
	for _, answer := range input.Answers {
		projected, err := projectAnswer(answer)
		if err != nil {
			return agent.SegmentStream{}, err
		}
		request.Responses = append(request.Responses, projected)
	}
	if input.Message != nil {
		content, err := r.projectInput(ctx, *input.Message)
		if err != nil {
			return agent.SegmentStream{}, err
		}
		request.Input = content
	}
	options, err := r.runCommandOptionsFor(input.CommandID)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	ack, events, err := r.runs.ResumeRun(ctx, request, options)
	if err != nil {
		return agent.SegmentStream{}, classifyError(err)
	}
	stream := agent.SegmentStream{}
	if ack != nil {
		stream.RunID, stream.SegmentID = ack.RunID, ack.SegmentID
		if ack.UserItemID != nil {
			stream.UserItemID = *ack.UserItemID
		}
	}
	if events != nil {
		stream.Events = projectEventStream(events, stream.SegmentID)
	}
	if ack == nil || events == nil {
		return agent.SegmentStream{}, agent.NewAcceptedMutationError(
			stream, runtimeContractViolation("resume run returned an incomplete stream"),
		)
	}
	if err := stream.ValidateResume(input.RunID, input.Message); err != nil {
		return agent.SegmentStream{}, agent.NewAcceptedMutationError(
			stream, runtimeContractViolation("resume run returned an invalid stream: %v", err),
		)
	}
	return stream, nil
}

func projectAnswer(value agent.InterruptAnswer) (protocol.InterruptResponse, error) {
	response := protocol.InterruptResponse{ItemID: value.ItemID}
	switch answer := value.Answer.(type) {
	case agent.ApprovalAnswer:
		response.Response.Type = protocol.InterruptResponseApproval
		response.Response.Decision = protocol.ApprovalDecision(answer.Decision)
		response.Response.Reason = answer.Reason
		if answer.ArgumentOverride != nil {
			arguments, err := answer.ArgumentOverride.Object()
			if err != nil {
				return protocol.InterruptResponse{}, fmt.Errorf("answer for item %s: %w", value.ItemID, err)
			}
			response.Response.EditedArgs = arguments
		}
		if answer.Remember != agent.RememberNone {
			response.Response.Remember = &protocol.RememberScope{Scope: protocol.RememberScopeKind(answer.Remember)}
		}
	case agent.QuestionAnswer:
		response.Response.Type = protocol.InterruptResponseAnswer
		response.Response.Answers = make([][]string, len(answer.Values))
		for index, answers := range answer.Values {
			response.Response.Answers[index] = slices.Clone(answers)
		}
	default:
		return protocol.InterruptResponse{}, fmt.Errorf("answer for item %s has unsupported type %T", value.ItemID, value.Answer)
	}
	return response, nil
}

func (r *Runtime) SubscribeRun(ctx context.Context, input agent.SubscribeRun) (agent.SegmentStream, error) {
	if err := input.Validate(); err != nil {
		return agent.SegmentStream{}, err
	}
	ack, events, err := r.runs.SubscribeRun(ctx, protocol.SubscribeRunRequest{
		RunID: input.RunID, SegmentID: input.SegmentID,
	}, r.subscriptionOptions(input.AfterEventID))
	if err != nil {
		return agent.SegmentStream{}, classifyError(err)
	}
	if ack == nil || events == nil {
		return agent.SegmentStream{}, runtimeContractViolation("subscribe run returned an incomplete stream")
	}
	stream := agent.SegmentStream{
		RunID: ack.RunID, SegmentID: ack.SegmentID, HeadEventID: ack.HeadEventID,
		Events: projectEventStream(events, ack.SegmentID),
	}
	if err := stream.ValidateSubscription(); err != nil {
		return agent.SegmentStream{}, runtimeContractViolation("subscribe run returned an invalid stream: %v", err)
	}
	if stream.RunID != input.RunID || stream.SegmentID != input.SegmentID {
		return agent.SegmentStream{}, runtimeContractViolation(
			"subscribe run returned segment %s/%s for %s/%s",
			stream.RunID, stream.SegmentID, input.RunID, input.SegmentID,
		)
	}
	return stream, nil
}

func (r *Runtime) CancelRun(ctx context.Context, input agent.CancelRun) (agent.RunCancellation, error) {
	if err := input.Validate(); err != nil {
		return agent.RunCancellation{}, err
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return agent.RunCancellation{}, err
	}
	result, err := r.runs.CancelRun(ctx, protocol.CancelRunRequest{RunID: input.RunID, Reason: input.Reason}, options)
	if err != nil {
		return agent.RunCancellation{}, classifyError(err)
	}
	if result == nil {
		return agent.RunCancellation{}, runtimeContractViolation("cancel run returned nil")
	}
	canceled, err := projectRun(result.Run)
	if err != nil {
		return agent.RunCancellation{}, runtimeContractViolation("cancel run returned an invalid run: %v", err)
	}
	var root agent.Run
	switch result.Type {
	case protocol.CancelRunRoot:
		if result.RootRun != nil {
			return agent.RunCancellation{}, runtimeContractViolation("root cancellation carries rootRun")
		}
		root = canceled.Clone()
	case protocol.CancelRunChild:
		if result.RootRun == nil {
			return agent.RunCancellation{}, runtimeContractViolation("child cancellation omits rootRun")
		}
		root, err = projectRun(*result.RootRun)
		if err != nil {
			return agent.RunCancellation{}, runtimeContractViolation("cancel run returned an invalid root: %v", err)
		}
	default:
		return agent.RunCancellation{}, runtimeContractViolation("cancel run returned unknown result type %q", result.Type)
	}
	projected := agent.RunCancellation{Canceled: canceled, Root: root}
	if err := projected.ValidateTarget(input.RunID); err != nil {
		return agent.RunCancellation{}, runtimeContractViolation("cancel run returned an invalid projection: %v", err)
	}
	return projected, nil
}

func (r *Runtime) SteerRun(ctx context.Context, input agent.SteerRun) error {
	if err := input.Validate(); err != nil {
		return err
	}
	content, err := r.projectInput(ctx, input.Message)
	if err != nil {
		return err
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return err
	}
	return classifyError(r.runs.SteerRun(ctx, protocol.SteerRunRequest{
		RunID: input.RunID, ExpectedSegmentID: input.SegmentID, Input: content,
	}, options))
}

func projectEventStream(source iter.Seq2[protocol.RunEvent, error], streamSegmentID string) agent.EventStream {
	return func(yield func(agent.RunEvent, error) bool) {
		for value, streamErr := range source {
			if streamErr != nil {
				yield(agent.RunEvent{}, classifyError(streamErr))
				return
			}
			projected, include, err := projectEvent(value)
			if err != nil {
				yield(agent.RunEvent{}, runtimeContractViolation("run stream returned an invalid event: %v", err))
				return
			}
			projected.StreamSegmentID = streamSegmentID
			if include && !yield(projected, nil) {
				return
			}
		}
	}
}
