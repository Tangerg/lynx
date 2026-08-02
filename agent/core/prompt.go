package core

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

// ChatMiddleware carries model middleware applied to process-scoped chat
// capabilities. Runtime constructors snapshot the value and both slices before
// retaining it.
type ChatMiddleware struct {
	CallMiddlewares   []chat.CallMiddleware
	StreamMiddlewares []chat.StreamMiddleware
}

// Empty reports whether m changes chat execution.
func (m *ChatMiddleware) Empty() bool {
	if m == nil {
		return true
	}
	return len(m.CallMiddlewares) == 0 && len(m.StreamMiddlewares) == 0
}

// PromptConfig configures one framework-managed model interaction. Its zero
// value uses the process model, action tools, and process tool-round limit.
type PromptConfig struct {
	System             string
	Options            *chat.Options
	Tools              []tool.Tool
	DisableActionTools bool
	MaxToolRounds      int
}

// Prompt runs one model interaction through the process tool loop and returns
// the final model text or direct-tool result.
func (pc *ProcessContext) Prompt(ctx context.Context, text string, config PromptConfig) (string, error) {
	call, err := pc.newPromptCall(ctx, text, config)
	if err != nil {
		return "", err
	}
	result, err := pc.Interact(ctx, Interaction{
		Request: call.request,
		Tools:   call.registry,
		Limits:  interaction.Limits{MaxRounds: call.maxRounds},
	})
	if err != nil {
		return "", err
	}
	// Prompt's whole contract is the final text, so it has nowhere to report a
	// partial result: a stop that the caller's own limits asked for still has to
	// come back as an error, but it names the bound instead of looking like a
	// missing event. Callers that want the partial work use Interact directly.
	if result.StopReason != interaction.StopNone {
		return "", fmt.Errorf("agent: prompt stopped before a final event: %s", result.StopReason)
	}
	if result.Final == nil {
		return "", errors.New("agent: prompt ended without a final event")
	}
	switch result.Final.Kind {
	case interaction.EventModelResponse:
		return result.Final.Response.Text(), nil
	case interaction.EventToolResult:
		return result.Final.ToolResult.Result, nil
	default:
		return "", fmt.Errorf("agent: prompt ended with unexpected event %q", result.Final.Kind)
	}
}

type promptCall struct {
	request   *chat.Request
	registry  *tool.Registry
	maxRounds int
}

func (pc *ProcessContext) newPromptCall(ctx context.Context, text string, config PromptConfig) (*promptCall, error) {
	if pc == nil {
		return nil, errors.New("agent: prompt: process context is nil")
	}
	resolved, err := pc.promptTools(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("agent: prompt: resolve tools: %w", err)
	}
	registry, err := tool.NewRegistry(resolved...)
	if err != nil {
		return nil, fmt.Errorf("agent: prompt: register tools: %w", err)
	}

	messages := make([]chat.Message, 0, 2)
	if config.System != "" {
		messages = append(messages, chat.NewSystemMessage(config.System))
	}
	messages = append(messages, chat.NewUserMessage(chat.NewTextPart(text)))
	request := &chat.Request{Messages: messages, Tools: registry.Definitions()}
	if config.Options != nil {
		request.Options = *config.Options
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("agent: prompt: validate request: %w", err)
	}

	maxRounds := pc.maxToolRounds
	if config.MaxToolRounds != 0 {
		maxRounds = config.MaxToolRounds
	}
	if maxRounds < 0 {
		return nil, errors.New("agent: prompt: max tool rounds must not be negative")
	}
	return &promptCall{
		request:   request,
		registry:  registry,
		maxRounds: maxRounds,
	}, nil
}

func (pc *ProcessContext) promptTools(ctx context.Context, config PromptConfig) ([]tool.Tool, error) {
	if config.DisableActionTools {
		return slices.Clone(config.Tools), nil
	}
	actionTools, err := pc.ActionTools(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]tool.Tool, 0, len(actionTools)+len(config.Tools))
	resolved = append(resolved, actionTools...)
	return append(resolved, config.Tools...), nil
}
