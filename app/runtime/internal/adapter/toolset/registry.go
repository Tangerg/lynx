package toolset

import (
	"context"
	"errors"
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

// NewRegistry returns a diagnostic registry over the assembled tool catalog.
// List snapshots metadata; Invoke calls a registered tool directly without an
// agent turn.
func NewRegistry(src *Resolver) (toolapp.Registry, error) {
	if src == nil {
		return nil, errors.New("toolset: tool source is required")
	}
	return &registry{src: src}, nil
}

type registry struct {
	src *Resolver
}

func (r *registry) List(ctx context.Context) ([]tool.Tool, error) {
	chatTools := r.src.toolsFor(ctx)
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

func (r *registry) Invoke(ctx context.Context, name, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("toolset: tool name must not be empty")
	}
	ctx, span := toolTracer.Start(ctx, "execute_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrGenAIToolName, name)))
	defer span.End()

	for _, candidate := range r.src.toolsFor(ctx) {
		if candidate.Definition().Name != name {
			continue
		}
		output, err := candidate.Call(ctx, arguments)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return output, err
	}
	err := fmt.Errorf("toolset: tool %q not registered", name)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return "", err
}
