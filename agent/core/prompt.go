package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
)

// ChatGuardrails carries engine-wide model middleware and the bounded tool-loop
// policy used by Prompt. Runtime constructors snapshot the value and both
// middleware slices before retaining it.
type ChatGuardrails struct {
	// BindConversation projects the framework's conversation identity into a
	// model-call context understood by host middleware. The host owns the
	// context protocol and any storage behind it.
	BindConversation func(context.Context, string) context.Context

	CallMiddlewares   []chat.CallMiddleware
	StreamMiddlewares []chat.StreamMiddleware

	// MaxToolRounds bounds Prompt tool execution. Zero selects the runner
	// default.
	MaxToolRounds int
}

// Empty reports whether g changes chat execution.
func (g *ChatGuardrails) Empty() bool {
	if g == nil {
		return true
	}
	return g.BindConversation == nil && len(g.CallMiddlewares) == 0 && len(g.StreamMiddlewares) == 0 && g.MaxToolRounds == 0
}

// PromptConfig configures one framework-managed model interaction. Its zero
// value uses the process model, action tools, and process tool-round limit.
type PromptConfig struct {
	System             string
	Options            *chat.Options
	Tools              []tools.Tool
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
		Model:   call.model,
		Request: call.request,
		Tools:   call.registry,
		Limits:  interaction.Limits{MaxRounds: call.maxRounds},
	})
	if err != nil {
		return "", err
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

// PromptJSON requests JSON matching T and decodes the final response.
func PromptJSON[T any](ctx context.Context, process *ProcessContext, text string, config PromptConfig) (T, error) {
	var output T
	if process == nil {
		return output, errors.New("agent: prompt JSON: process context is nil")
	}
	schema, err := pkgjson.StringDefSchemaOf(output)
	if err != nil {
		return output, fmt.Errorf("agent: prompt JSON: derive schema: %w", err)
	}
	prompt := text + "\n\nReply with only JSON matching this JSON SCHEMA:\n" + schema
	response, err := process.Prompt(ctx, prompt, config)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal([]byte(response), &output); err != nil {
		return output, fmt.Errorf("agent: prompt JSON: decode response: %w", err)
	}
	return output, nil
}

type promptCall struct {
	model     chat.Model
	request   *chat.Request
	registry  *tools.Registry
	maxRounds int
}

func (pc *ProcessContext) newPromptCall(ctx context.Context, text string, config PromptConfig) (*promptCall, error) {
	if pc == nil {
		return nil, errors.New("agent: prompt: process context is nil")
	}
	capability, err := pc.Chat()
	if err != nil {
		return nil, fmt.Errorf("agent: prompt: %w", err)
	}
	resolved, err := pc.promptTools(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("agent: prompt: resolve tools: %w", err)
	}
	registry, err := tools.NewRegistry(resolved...)
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
		model:     capability.Model,
		request:   request,
		registry:  registry,
		maxRounds: maxRounds,
	}, nil
}

func (pc *ProcessContext) promptTools(ctx context.Context, config PromptConfig) ([]tools.Tool, error) {
	if config.DisableActionTools {
		return slices.Clone(config.Tools), nil
	}
	actionTools, err := pc.ActionTools(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]tools.Tool, 0, len(actionTools)+len(config.Tools))
	resolved = append(resolved, actionTools...)
	return append(resolved, config.Tools...), nil
}
