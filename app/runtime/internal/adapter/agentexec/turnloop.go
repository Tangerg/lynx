package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
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

// streamIdleMiddleware keeps the product's stream-stall policy in the chat
// pipeline. Agent still owns stream accumulation, interaction events, and
// resource accounting.
func streamIdleMiddleware(idle time.Duration) chat.StreamMiddleware {
	return func(next chat.Streamer) chat.Streamer {
		return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				streamCtx, keepAlive, stop := modelStreamContext(ctx, idle)
				var streamErr error
				for response, err := range next.Stream(streamCtx, request) {
					if err != nil {
						streamErr = err
						break
					}
					keepAlive()
					if !yield(response, nil) {
						_ = stop()
						return
					}
				}
				if cause := stop(); cause != nil {
					yield(nil, cause)
					return
				}
				if streamErr != nil {
					yield(nil, streamErr)
				}
			}
		})
	}
}

// deferredToolProvider is implemented by a meta-tool (search_tools) that keeps
// some resolvable tools out of the model's initial manifest and surfaces them on
// demand. The turn withholds these names from the advertised toolset while the
// registry keeps them executable, so the meta-tool can promote a chosen tool
// mid-loop (agent/toolloop PromoteTools) and the model calls it directly next round.
type deferredToolProvider interface {
	DeferredToolNames() []string
}

type preparedTurn struct {
	registry *tools.Registry
	request  *chat.Request
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
// framework-managed interaction boundary. Agent owns tool iteration,
// checkpointing, suspension, aggregate resource counters, and limit
// enforcement; Runtime owns the model-level accounting projection.
func (e *Engine) runTurn(ctx context.Context, pc *core.ProcessContext, message string, images []*media.Media, options *chat.Options) (TurnOutput, error) {
	ledger, err := usageLedgerFrom(pc.Dependencies())
	if err != nil {
		return TurnOutput{}, err
	}
	prepared, err := e.prepareTurn(ctx, pc, message, images, options)
	if err != nil {
		return TurnOutput{}, err
	}

	var observer toolObserver
	if observation := observationFrom(pc.Dependencies()); observation != nil {
		observer = observation.target
	}
	// partial retains only the text needed when the framework deliberately
	// stops before a tagged final response (budget / step limit). Normal
	// completion always reads result.Final below.
	var partial strings.Builder
	stream := func(response *chat.Response) {
		choice := response.First()
		if choice == nil || choice.Message == nil {
			return
		}
		for _, part := range choice.Message.Parts {
			switch part.Kind {
			case chat.PartReasoning:
				if observer != nil && part.Text != "" {
					observer.OnReasoningDelta(processRef(pc.Process()), part.Text)
				}
			case chat.PartText:
				partial.WriteString(part.Text)
				if observer != nil {
					observer.OnMessageDelta(processRef(pc.Process()), part.Text)
				}
			}
		}
	}

	result, err := pc.Interact(ctx, core.Interaction{
		Request: prepared.request,
		Tools:   prepared.registry,
		Stream:  stream,
	})
	if err != nil {
		return TurnOutput{}, err
	}
	return turnOutputFromInteraction(ledger, result, partial.String())
}

func (e *Engine) prepareTurn(ctx context.Context, pc *core.ProcessContext, message string, images []*media.Media, options *chat.Options) (preparedTurn, error) {
	actionTools, err := pc.ActionTools(ctx)
	if err != nil {
		return preparedTurn{}, fmt.Errorf("agentexec: resolve action tools: %w", err)
	}
	registry, err := tools.NewRegistry(actionTools...)
	if err != nil {
		return preparedTurn{}, fmt.Errorf("agentexec: register action tools: %w", err)
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
		return preparedTurn{}, fmt.Errorf("agentexec: turn request: %w", err)
	}
	return preparedTurn{registry: registry, request: request}, nil
}

func turnOutputFromInteraction(ledger *usageLedger, result interaction.Result, partial string) (TurnOutput, error) {
	switch result.StopReason {
	case agent.InteractionStopBudget:
		return ledger.output(partial, StopReasonBudget), nil
	case agent.InteractionStopSteps:
		return ledger.output(partial, StopReasonSteps), nil
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
		return ledger.output(result.Final.Response.Text(), StopReasonNone), nil
	case agent.InteractionEventToolResult:
		if result.Final.ToolResult == nil {
			return TurnOutput{}, errors.New("agentexec: final tool result event has no result")
		}
		return ledger.output(result.Final.ToolResult.Result, StopReasonNone), nil
	default:
		return TurnOutput{}, fmt.Errorf("agentexec: unexpected final interaction event %q", result.Final.Kind)
	}
}
