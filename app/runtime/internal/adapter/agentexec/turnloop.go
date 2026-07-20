package agentexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/tools"
)

const llmIdleTimeout = 5 * time.Minute

var (
	errModelStreamIdleTimeout = errors.New("agentexec: model stream idle timeout")
	errModelStreamCompleted   = errors.New("agentexec: model stream completed")
)

func modelStreamContext(parent context.Context, idle time.Duration) (ctx context.Context, keepAlive func(), stop func() error) {
	ctx, cancel := context.WithCancelCause(parent)
	var (
		mu         sync.Mutex
		timer      *time.Timer
		generation uint64
		finished   bool
		winner     error
	)

	armLocked := func() {
		generation++
		current := generation
		timer = time.AfterFunc(idle, func() {
			mu.Lock()
			defer mu.Unlock()
			if finished || current != generation {
				return
			}
			finished = true
			generation++
			if cause := context.Cause(parent); cause != nil {
				winner = cause
				return
			}
			winner = errModelStreamIdleTimeout
			cancel(errModelStreamIdleTimeout)
		})
	}

	mu.Lock()
	armLocked()
	mu.Unlock()

	return ctx, func() {
			mu.Lock()
			defer mu.Unlock()
			if finished {
				return
			}
			if cause := context.Cause(parent); cause != nil {
				finished = true
				winner = cause
				generation++
				if timer != nil {
					timer.Stop()
				}
				return
			}
			if timer != nil {
				timer.Stop()
			}
			armLocked()
		}, func() error {
			mu.Lock()
			defer mu.Unlock()
			if finished {
				return winner
			}
			finished = true
			generation++
			if timer != nil {
				timer.Stop()
			}
			if cause := context.Cause(parent); cause != nil {
				winner = cause
				return winner
			}
			cancel(errModelStreamCompleted)
			return nil
		}
}

// streamingModel retains provider streaming for the UI while presenting the
// complete response required by the framework's synchronous interaction port.
type streamingModel struct {
	streamer    chat.Streamer
	chunk       func(*chat.Response)
	idleTimeout time.Duration
}

func (m streamingModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	streamCtx, keepAlive, stop := modelStreamContext(ctx, m.idleTimeout)
	var accumulator chat.ResponseAccumulator
	seen := false
	for response, err := range m.streamer.Stream(streamCtx, request) {
		if err != nil {
			if cause := stop(); cause != nil {
				return nil, cause
			}
			return nil, err
		}
		if response == nil {
			if cause := stop(); cause != nil {
				return nil, cause
			}
			return nil, errors.New("agentexec: chat stream yielded a nil response")
		}
		if err := accumulator.Add(response); err != nil {
			if cause := stop(); cause != nil {
				return nil, cause
			}
			return nil, fmt.Errorf("agentexec: accumulate chat stream: %w", err)
		}
		seen = true
		keepAlive()
		if m.chunk != nil {
			m.chunk(response)
		}
	}
	if cause := stop(); cause != nil {
		return nil, cause
	}
	if !seen {
		return nil, errors.New("agentexec: chat stream ended without a response")
	}
	return accumulator.Response(), nil
}

// deferredToolProvider is implemented by a meta-tool (search_tools) that keeps
// some resolvable tools out of the model's initial manifest and surfaces them on
// demand. The turn withholds these names from the advertised toolset while the
// registry keeps them executable, so the meta-tool can promote a chosen tool
// mid-loop (agent/toolloop PromoteTools) and the model calls it directly next round.
type deferredToolProvider interface {
	DeferredToolNames() []string
}

// advertisedTools projects the executable registry into the model-facing tool
// manifest, excluding every tool a deferred-tool provider withholds. The
// withheld tools stay in the registry (resolvable) so a mid-loop promotion can
// advertise them; they are simply absent from the initial round's schema.
func advertisedTools(actionTools []tools.Tool, registry *tools.Registry) []chat.ToolDefinition {
	definitions := registry.Definitions()
	var deferred map[string]struct{}
	for _, tool := range actionTools {
		provider, ok := tool.(deferredToolProvider)
		if !ok {
			continue
		}
		for _, name := range provider.DeferredToolNames() {
			if deferred == nil {
				deferred = make(map[string]struct{})
			}
			deferred[name] = struct{}{}
		}
	}
	if len(deferred) == 0 {
		return definitions
	}
	advertised := definitions[:0]
	for _, def := range definitions {
		if _, hidden := deferred[def.Name]; hidden {
			continue
		}
		advertised = append(advertised, def)
	}
	return advertised
}

// runTurn supplies app-specific streaming and pricing adapters to the
// framework-managed interaction boundary. The Agent framework owns tool
// iteration, checkpointing, suspension, usage recording, and budget/step
// enforcement.
func (e *Engine) runTurn(ctx context.Context, pc *core.ProcessContext, provider, message string, images []*media.Media, options *chat.Options, budget accounting.Budget) (TurnOutput, error) {
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
	if message != "" {
		parts = append(parts, chat.NewTextPart(message))
	}
	for _, image := range images {
		parts = append(parts, chat.NewMediaPart(image))
	}
	messages := []chat.Message{chat.NewSystemMessage(e.systemPrompt(ctx))}
	if recall, ok := e.recalledMemories(ctx, message); ok {
		messages = append(messages, recall)
	}
	messages = append(messages, chat.NewUserMessage(parts...))
	request := &chat.Request{
		Messages: messages,
		Tools:    advertisedTools(actionTools, registry),
	}
	if options != nil {
		request.Options = options.Clone()
	}
	if err := request.Validate(); err != nil {
		return TurnOutput{}, fmt.Errorf("agentexec: turn request: %w", err)
	}

	observation := observationFrom(pc.Dependencies())
	var observer toolObserver
	if observation != nil {
		observer = observation.target
	}
	// partial retains only the text needed when the framework deliberately
	// stops before a tagged final response (budget / step limit). Normal
	// completion always reads result.Final below.
	var partial strings.Builder
	model := streamingModel{
		streamer:    capability.Streamer,
		idleTimeout: e.modelStreamIdleTimeout,
		chunk: func(response *chat.Response) {
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
					partial.WriteString(part.Text)
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
			MaxTokens:     budget.MaxTokens,
			MaxCostUSD:    budget.MaxCostUSD,
			MaxModelCalls: budget.MaxSteps,
		},
		Attribute: e.modelAttribution(provider),
		Observe: func(_ context.Context, boundary agent.InteractionEvent) error {
			if observation != nil {
				switch boundary.Kind {
				case agent.InteractionEventToolCall:
					if boundary.ToolCall != nil {
						observation.begin(pc.Process().ID(), boundary.Round, *boundary.ToolCall)
					}
				case agent.InteractionEventToolResult:
					if boundary.ToolResult != nil {
						observation.result(pc.Process().ID(), boundary.Round, *boundary.ToolResult)
					}
				}
			}
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
		return turnOutput(pc, partial.String(), StopReasonBudget), nil
	case agent.InteractionStopSteps:
		return turnOutput(pc, partial.String(), StopReasonSteps), nil
	case agent.InteractionStopNone:
	default:
		return TurnOutput{}, fmt.Errorf("agentexec: unexpected interaction stop reason %q", result.StopReason)
	}
	if result.Final == nil {
		return TurnOutput{}, errors.New("agentexec: managed interaction ended without a final event")
	}
	switch result.Final.Kind {
	case agent.InteractionEventModelResponse:
		if result.Final.Response == nil {
			return TurnOutput{}, errors.New("agentexec: final model response event has no response")
		}
		return turnOutput(pc, result.Final.Response.Text(), StopReasonNone), nil
	case agent.InteractionEventToolResult:
		if result.Final.ToolResult == nil {
			return TurnOutput{}, errors.New("agentexec: final tool result event has no result")
		}
		return turnOutput(pc, result.Final.ToolResult.Result, StopReasonNone), nil
	default:
		return TurnOutput{}, fmt.Errorf("agentexec: unexpected final interaction event %q", result.Final.Kind)
	}
}
