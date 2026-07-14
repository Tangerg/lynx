package agentexec

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools"
)

// toolTracer spans direct (out-of-turn) tool invocations. Tool calls the model
// makes during a chat turn are traced at the target chat/tool adapter boundaries;
// this covers only the diagnostic Invoke path. Tool name key follows the
// gen_ai semconv. No-op until a provider is set.
var toolTracer = otel.Tracer("lynx/lyra/tool")

const attrGenAIToolName = "gen_ai.tool.name"

// toolSource is the narrow surface the registry consumes: a snapshot of the
// currently-registered chat tools. *Engine satisfies it implicitly via its
// Tools() accessor; tests pass a stub that returns a fixed slice without needing
// a real platform.
type toolSource interface {
	Tools() []tools.Tool
}

// NewToolRegistry returns the engine-backed [tool.Registry]: List snapshots the
// registered tools; Invoke routes by tool name to the registered tool's Call
// method (no agent loop involved — direct synchronous invocation, traced here at
// the adapter boundary).
func NewToolRegistry(src toolSource) (tool.Registry, error) {
	if src == nil {
		return nil, errors.New("agentexec: tool source is required")
	}
	return &toolRegistry{src: src}, nil
}

// toolRegistry is the engine-backed registered-tool directory. The source is
// typically the engine but can be any tool snapshot provider in tests.
type toolRegistry struct {
	src toolSource
}

func (r *toolRegistry) List(_ context.Context) ([]tool.Tool, error) {
	chatTools := r.src.Tools()
	out := make([]tool.Tool, 0, len(chatTools))
	for _, t := range chatTools {
		def := t.Definition()
		out = append(out, tool.Tool{
			Name:        def.Name,
			Description: def.Description,
			Schema:      string(def.InputSchema),
			SafetyClass: tool.SafetyClassFor(def.Name),
		})
	}
	return out, nil
}

func (r *toolRegistry) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("agentexec: tool name must not be empty")
	}
	ctx, span := toolTracer.Start(ctx, "execute_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrGenAIToolName, name)))
	defer span.End()

	for _, t := range r.src.Tools() {
		if t.Definition().Name == name {
			out, err := t.Call(ctx, arguments)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return out, err
		}
	}
	err := fmt.Errorf("agentexec: tool %q not registered", name)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return "", err
}
