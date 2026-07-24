package toolset

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	toolapp "github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

var toolTracer = otel.Tracer("lynx/lyra/tool")

const attrGenAIToolName = "gen_ai.tool.name"

// NewDiagnosticRegistry returns the explicitly direct-invocable diagnostic
// catalog. It deliberately does not reuse the agent resolver: agent tools may
// require a process, session, approval flow, or model loop that does not exist
// for a client-driven call.
func NewDiagnosticRegistry() toolapp.Registry { return registry{} }

type registry struct{}

func (registry) List(context.Context) ([]tool.Tool, error) {
	chatTools := directTools("")
	out := make([]tool.Tool, 0, len(chatTools))
	for _, candidate := range chatTools {
		definition := candidate.Definition()
		schema, err := tool.ParseSchema(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("toolset: decode input schema for tool %q: %w", definition.Name, err)
		}
		out = append(out, tool.Tool{
			Name:        definition.Name,
			Description: definition.Description,
			Schema:      schema,
			SafetyClass: tool.SafetyClassFor(definition.Name),
		})
	}
	return out, nil
}

func (registry) Invoke(ctx context.Context, root, name, arguments string) (tool.Result, error) {
	if name == "" {
		return tool.Result{}, fmt.Errorf("toolset: direct tool name must not be empty")
	}
	ctx, span := toolTracer.Start(ctx, "execute_direct_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrGenAIToolName, name)))
	defer span.End()

	arguments, err := normalizeDirectArguments(root, name, arguments)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return tool.Result{}, err
	}
	for _, candidate := range directTools(root) {
		if candidate.Definition().Name != name {
			continue
		}
		output, err := candidate.Call(ctx, arguments)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return tool.Result{}, err
		}
		return directResult(output), nil
	}
	err = fmt.Errorf("toolset: direct tool %q is not registered", name)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return tool.Result{}, err
}
