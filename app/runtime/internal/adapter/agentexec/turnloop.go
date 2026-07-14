package agentexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/chatclient"
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

// streamingModel adapts a target chat client to the synchronous model port the
// target Event Runner consumes. It retains true provider streaming for the UI,
// while ResponseAccumulator produces the complete response the runner needs
// to inspect for tool calls.
type streamingModel struct {
	client *chatclient.Client
	chunk  func(*chat.Response)
	finish func(*chat.Response)
}

func (m streamingModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	var accumulator chat.ResponseAccumulator
	seen := false
	for response, err := range m.client.Stream(ctx, request) {
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
	response := accumulator.Response()
	if m.finish != nil {
		m.finish(response)
	}
	return response, nil
}

// runTurn builds ordinary target protocol values, keeps executable tools in a
// sibling Registry, and drives the target Event Runner. HITL persists a
// Checkpoint on the process blackboard and resumes at the pending tool.
func (e *Engine) runTurn(ctx context.Context, pc *core.ProcessContext, provider, message string, images []*media.Media, options *chat.Options, budget accounting.Budget) (TurnOutput, error) {
	ctx, keepAlive, stop := stallContext(ctx, llmIdleTimeout)
	defer stop()

	client, err := pc.Chat()
	if err != nil {
		return TurnOutput{}, err
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
		request.Options = cloneChatOptions(*options)
	}
	if err := request.Validate(); err != nil {
		return TurnOutput{}, fmt.Errorf("agentexec: turn request: %w", err)
	}

	observer := observerFrom(pc.Options)
	var accumulated strings.Builder
	var cumulative accounting.TokenUsage
	var cumulativeCost float64
	for _, invocation := range pc.Process.LLMInvocations() {
		cumulative.Add(tokenUsageOf(invocation))
		cumulativeCost += invocation.CostUSD
	}
	model := streamingModel{
		client: client,
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
		finish: func(response *chat.Response) {
			usage := response.Usage
			if usage.TotalTokens() == 0 && response.Model == "" {
				return
			}
			invocation := e.invocationFrom(provider, response.Model, &usage)
			pc.RecordLLMInvocation(ctx, invocation)
			cumulative.Add(tokenUsageOf(invocation))
			cumulativeCost += invocation.CostUSD
			if observer != nil {
				observer.OnUsage(cumulative, cumulativeCost, invocation.PromptTokens)
			}
		},
	}

	runner, err := toolloop.NewRunner(model, toolloop.RunnerConfig{})
	if err != nil {
		return TurnOutput{}, err
	}
	checkpoints := checkpointStore{bb: pc.Blackboard}
	checkpoint, resumed, err := checkpoints.Load()
	if err != nil {
		return TurnOutput{}, err
	}

	steps := 0
	var sequence func(func(toolloop.Event, error) bool)
	if resumed {
		// Resume starts a fresh action execution, so restore the observable
		// reply and completed-round count that lived in the paused runner.
		for i := range checkpoint.Request.Messages {
			if checkpoint.Request.Messages[i].Role == chat.RoleAssistant {
				accumulated.WriteString(checkpoint.Request.Messages[i].Text())
			}
		}
		accumulated.WriteString(checkpoint.Response.Text())
		steps = checkpoint.Round - 1
		checkpoints.Clear()
		sequence = runner.Resume(ctx, checkpoint, registry, toolloop.Resume{ID: checkpoint.ID})
	} else {
		invocation, err := toolloop.NewInvocation(request, registry)
		if err != nil {
			return TurnOutput{}, err
		}
		sequence = runner.Run(ctx, invocation)
	}

	completedToolRound := false
	for event, runErr := range sequence {
		if runErr != nil {
			return TurnOutput{}, runErr
		}
		switch event.Kind {
		case toolloop.EventModelRequest:
			if !completedToolRound {
				continue
			}
			completedToolRound = false
			costUSD, tokens, _ := pc.Process.Usage()
			if budget.UsageExceeded(int64(tokens), costUSD) {
				return turnOutput(pc, accumulated.String(), true), nil
			}
			steps++
			if budget.StepsExceeded(steps) {
				output := turnOutput(pc, accumulated.String(), false)
				output.StoppedOnSteps = true
				return output, nil
			}
		case toolloop.EventToolResult:
			completedToolRound = true
			if event.Final && event.ToolResult != nil {
				return turnOutput(pc, event.ToolResult.Result, false), nil
			}
		case toolloop.EventModelResponse:
			if event.Final {
				return turnOutput(pc, accumulated.String(), false), nil
			}
		case toolloop.EventPause:
			if event.Pause == nil || event.Pause.Checkpoint == nil {
				return TurnOutput{}, errors.New("agentexec: tool loop paused without a checkpoint")
			}
			if err := checkpoints.Save(event.Pause.Checkpoint); err != nil {
				return TurnOutput{}, err
			}
			interrupt, ok := takePendingInterrupt(pc.Blackboard)
			if !ok {
				return TurnOutput{}, errors.New("agentexec: tool loop paused without a HITL awaitable")
			}
			return TurnOutput{}, interrupt
		}
	}
	return TurnOutput{}, errors.New("agentexec: tool loop ended without a terminal event")
}

func cloneChatOptions(options chat.Options) chat.Options {
	clone := options
	clone.Stop = append([]string(nil), options.Stop...)
	return clone
}
