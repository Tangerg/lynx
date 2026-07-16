package agentexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/tools"
)

const llmIdleTimeout = 5 * time.Minute

func stallContext(parent context.Context, idle time.Duration) (ctx context.Context, keepAlive, stop func()) {
	ctx, cancel := context.WithCancel(parent)
	timer := time.AfterFunc(idle, cancel)
	return ctx, func() { timer.Reset(idle) }, func() { timer.Stop(); cancel() }
}

// streamingModel retains provider streaming for the UI while presenting the
// complete response required by the framework's synchronous interaction port.
type streamingModel struct {
	streamer chat.Streamer
	chunk    func(*chat.Response)
}

func (m streamingModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	var accumulator chat.ResponseAccumulator
	seen := false
	for response, err := range m.streamer.Stream(ctx, request) {
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("agentexec: chat stream yielded a nil response")
		}
		seen = true
		if m.chunk != nil {
			m.chunk(response)
		}
		if err := accumulator.Add(response); err != nil {
			return nil, fmt.Errorf("agentexec: accumulate chat stream: %w", err)
		}
	}
	if !seen {
		return nil, errors.New("agentexec: chat stream ended without a response")
	}
	return accumulator.Response(), nil
}

// runTurn supplies app-specific streaming and pricing adapters to the
// framework-managed interaction boundary. Runtime owns tool iteration,
// checkpointing, suspension, usage recording, and budget/step enforcement.
func (e *Engine) runTurn(ctx context.Context, pc *core.ProcessContext, provider, message string, images []*media.Media, options *chat.Options, budget accounting.Budget) (TurnOutput, error) {
	ctx, keepAlive, stop := stallContext(ctx, llmIdleTimeout)
	defer stop()

	capability, err := pc.Chat()
	if err != nil {
		return TurnOutput{}, err
	}
	if capability.Streamer == nil {
		return TurnOutput{}, errors.New("agentexec: configured chat capability does not support streaming")
	}
	actionTools, err := pc.ActionTools(ctx)
	if err != nil {
		return TurnOutput{}, fmt.Errorf("agentexec: resolve action tools: %w", err)
	}
	registry, err := tools.NewRegistry(actionTools...)
	if err != nil {
		return TurnOutput{}, fmt.Errorf("agentexec: register action tools: %w", err)
	}

	parts := make([]chat.Part, 0, len(images)+1)
	parts = append(parts, chat.NewTextPart(message))
	for _, image := range images {
		parts = append(parts, chat.NewMediaPart(image))
	}
	request := &chat.Request{
		Messages: []chat.Message{
			chat.NewSystemMessage(e.SystemPrompt(ctx)),
			chat.NewUserMessage(parts...),
		},
		Tools: registry.Definitions(),
	}
	if options != nil {
		request.Options = options.Clone()
	}
	if err := request.Validate(); err != nil {
		return TurnOutput{}, fmt.Errorf("agentexec: turn request: %w", err)
	}

	observer := observerFrom(pc.Dependencies())
	var accumulated strings.Builder
	model := streamingModel{
		streamer: capability.Streamer,
		chunk: func(response *chat.Response) {
			keepAlive()
			choice := response.First()
			if choice == nil || choice.Message == nil {
				return
			}
			for _, part := range choice.Message.Parts {
				switch part.Kind {
				case chat.PartReasoning:
					if observer != nil && part.Text != "" {
						observer.OnReasoningDelta(part.Text)
					}
				case chat.PartText:
					accumulated.WriteString(part.Text)
					if observer != nil {
						observer.OnMessageDelta(part.Text)
					}
				}
			}
		},
	}

	result, err := pc.Interact(ctx, core.Interaction{
		Model:   model,
		Request: request,
		Tools:   registry,
		Limits: agent.InteractionLimits{
			MaxTokens:  budget.MaxTokens,
			MaxCostUSD: budget.MaxCostUSD,
			MaxSteps:   budget.MaxSteps,
		},
		Attribute: e.modelAttribution(provider),
		Observe: func(_ context.Context, boundary agent.InteractionEvent) error {
			if observer != nil && boundary.Kind == agent.InteractionEventModelResponse &&
				(boundary.Response.Usage.TotalTokens() != 0 || boundary.Response.Model != "") {
				var cumulative accounting.TokenUsage
				var cumulativeCost float64
				for _, invocation := range pc.Process().ModelCalls() {
					cumulative.Add(tokenUsageOf(invocation))
					cumulativeCost += invocation.CostUSD
				}
				observer.OnUsage(cumulative, cumulativeCost, boundary.Response.Usage.InputTokens)
			}
			return nil
		},
	})
	if err != nil {
		return TurnOutput{}, err
	}
	switch result.StopReason {
	case agent.InteractionStopBudget:
		return turnOutput(pc, accumulated.String(), true), nil
	case agent.InteractionStopSteps:
		output := turnOutput(pc, accumulated.String(), false)
		output.StoppedOnSteps = true
		return output, nil
	}
	if result.Final == nil {
		return TurnOutput{}, errors.New("agentexec: managed interaction ended without a final event")
	}
	switch result.Final.Kind {
	case agent.InteractionEventModelResponse:
		return turnOutput(pc, accumulated.String(), false), nil
	case agent.InteractionEventToolResult:
		return turnOutput(pc, result.Final.ToolResult.Result, false), nil
	default:
		return TurnOutput{}, fmt.Errorf("agentexec: unexpected final interaction event %q", result.Final.Kind)
	}
}
