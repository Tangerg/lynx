package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
)

// PromptRunner is the ergonomic agent-layer entry point for one LLM request.
// It builds ordinary core/chat protocol values and keeps executable tools in a
// tools.Registry beside the request. The runtime-owned ToolLoopRunner drives
// tool calls when any tools are present.
type PromptRunner struct {
	pc *ProcessContext

	system          string
	options         *chat.Options
	extraTools      []tools.Tool
	skipActionTools bool
	maxToolRounds   int
}

// PromptRunner constructs fresh single-call configuration bound to pc.
func (pc *ProcessContext) PromptRunner() *PromptRunner { return &PromptRunner{pc: pc} }

// WithSystem sets the optional system message.
func (r *PromptRunner) WithSystem(prompt string) *PromptRunner {
	r.system = prompt
	return r
}

// WithOptions sets request options. The value is copied while building the
// request; nil leaves provider/client defaults untouched.
func (r *PromptRunner) WithOptions(options *chat.Options) *PromptRunner {
	if options != nil {
		copy := *options
		r.options = &copy
	}
	return r
}

// WithTools appends executable tools to the action's resolved tool set.
func (r *PromptRunner) WithTools(values ...tools.Tool) *PromptRunner {
	r.extraTools = append(r.extraTools, values...)
	return r
}

// WithoutActionTools uses only tools supplied through WithTools.
func (r *PromptRunner) WithoutActionTools() *PromptRunner {
	r.skipActionTools = true
	return r
}

// WithMaxToolRounds overrides the process guardrail for this call. Zero uses
// the process or runner default; negative values fail during Generate/Stream.
func (r *PromptRunner) WithMaxToolRounds(rounds int) *PromptRunner {
	r.maxToolRounds = rounds
	return r
}

type promptCall struct {
	client    *chatclient.Client
	request   *chat.Request
	registry  *tools.Registry
	toolCount int
	maxRounds int
}

func (r *PromptRunner) build(ctx context.Context, userPrompt string) (*promptCall, error) {
	if r == nil || r.pc == nil {
		return nil, errors.New("agent.PromptRunner: ProcessContext is nil")
	}
	client, err := r.pc.Chat()
	if err != nil {
		return nil, fmt.Errorf("agent.PromptRunner: %w", err)
	}

	resolved, err := r.resolveTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent.PromptRunner: resolve tools: %w", err)
	}
	registry, err := tools.NewRegistry(resolved...)
	if err != nil {
		return nil, fmt.Errorf("agent.PromptRunner: register tools: %w", err)
	}

	messages := make([]chat.Message, 0, 2)
	if r.system != "" {
		messages = append(messages, chat.NewSystemMessage(r.system))
	}
	messages = append(messages, chat.NewUserMessage(chat.NewTextPart(userPrompt)))
	request := &chat.Request{Messages: messages, Tools: registry.Definitions()}
	if r.options != nil {
		request.Options = *r.options
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("agent.PromptRunner: request: %w", err)
	}

	maxRounds := 0
	if r.pc.guardrails != nil {
		maxRounds = r.pc.guardrails.MaxToolRounds
	}
	if r.maxToolRounds != 0 {
		maxRounds = r.maxToolRounds
	}
	if maxRounds < 0 {
		return nil, errors.New("agent.PromptRunner: MaxToolRounds must not be negative")
	}
	return &promptCall{
		client: client, request: request,
		registry: registry, toolCount: len(resolved), maxRounds: maxRounds,
	}, nil
}

func (r *PromptRunner) resolveTools(ctx context.Context) ([]tools.Tool, error) {
	if r.skipActionTools {
		return slices.Clone(r.extraTools), nil
	}
	actionTools, err := r.pc.ActionTools(ctx)
	if err != nil {
		return nil, err
	}
	combined := make([]tools.Tool, 0, len(actionTools)+len(r.extraTools))
	combined = append(combined, actionTools...)
	combined = append(combined, r.extraTools...)
	return combined, nil
}

// Generate returns the final model text, or the final direct-tool result when
// every tool in the last round is marked with toolloop.Direct.
func (r *PromptRunner) Generate(ctx context.Context, userPrompt string) (string, error) {
	call, err := r.build(ctx, userPrompt)
	if err != nil {
		return "", err
	}
	return r.generate(ctx, call)
}

func (r *PromptRunner) generate(ctx context.Context, call *promptCall) (string, error) {
	if call.toolCount == 0 {
		response, err := call.client.Call(ctx, call.request)
		if err != nil {
			return "", err
		}
		return response.Text(), nil
	}

	if r.pc.runToolLoop == nil {
		return "", errors.New("agent.PromptRunner: tool loop runner is not configured")
	}
	return r.pc.runToolLoop(ctx, call.client, call.request, call.registry, call.maxRounds)
}

// Stream yields provider text deltas when no tools are configured. A tool loop
// is synchronous by contract, so tool-enabled calls yield their final text as
// one item instead of pretending that a non-streaming loop is streaming.
func (r *PromptRunner) Stream(ctx context.Context, userPrompt string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		call, err := r.build(ctx, userPrompt)
		if err != nil {
			yield("", err)
			return
		}
		if call.toolCount != 0 {
			text, err := r.generate(ctx, call)
			yield(text, err)
			return
		}
		for response, streamErr := range call.client.Stream(ctx, call.request) {
			if !yield(response.Text(), streamErr) {
				return
			}
			if streamErr != nil {
				return
			}
		}
	}
}

// GenerateObject asks for JSON matching T and decodes the final reply.
func GenerateObject[T any](ctx context.Context, runner *PromptRunner, userPrompt string) (T, error) {
	var output T
	if runner == nil {
		return output, errors.New("agent.GenerateObject: runner is nil")
	}
	schema, err := pkgjson.StringDefSchemaOf(output)
	if err != nil {
		return output, fmt.Errorf("agent.GenerateObject: derive schema: %w", err)
	}
	prompt := userPrompt + "\n\nReply with only JSON matching this JSON SCHEMA:\n" + schema
	text, err := runner.Generate(ctx, prompt)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return output, fmt.Errorf("agent.GenerateObject: decode response: %w", err)
	}
	return output, nil
}
