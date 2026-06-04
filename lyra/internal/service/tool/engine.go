package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// toolTracer spans direct (out-of-turn) tool invocations. Tool calls the
// model makes during a chat turn are already traced by core/model/chat
// + the mcp module; this covers only the diagnostic Invoke path. Tool
// name key follows the gen_ai semconv. No-op until a provider is set.
var toolTracer = otel.Tracer("lynx/lyra/tool")

const attrGenAIToolName = "gen_ai.tool.name"

// Source is the narrow surface tool.Service consumes: just a
// snapshot of the currently-registered chat tools. *engine.Engine
// satisfies it implicitly via its Tools() accessor; tests pass a
// stub that returns a fixed slice without needing a real platform.
type Source interface {
	Tools() []chat.Tool
}

// New returns the [Service] implementation backed by src. List
// snapshots the registered tools; Invoke routes by tool name to
// the registered tool's Call method (no agent loop involved —
// direct synchronous invocation).
func New(src Source) Service {
	if src == nil {
		panic("tool: source is required")
	}
	return &engineBacked{src: src}
}

// engineBacked is the single Service implementation today. The
// "engine-backed" label is descriptive — the source is typically
// the engine but could be any Source (tests, mocks).
type engineBacked struct {
	src Source
}

func (s *engineBacked) List(_ context.Context) ([]Tool, error) {
	chatTools := s.src.Tools()
	out := make([]Tool, 0, len(chatTools))
	for _, t := range chatTools {
		def := t.Definition()
		out = append(out, Tool{
			Name:        def.Name,
			Description: def.Description,
			Schema:      def.InputSchema,
			SafetyClass: defaultSafetyClass(def.Name),
		})
	}
	return out, nil
}

func (s *engineBacked) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("tool: name must not be empty")
	}
	ctx, span := toolTracer.Start(ctx, "execute_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrGenAIToolName, name)))
	defer span.End()

	for _, t := range s.src.Tools() {
		if t.Definition().Name == name {
			slog.InfoContext(ctx, "tool invoked directly", attrGenAIToolName, name)
			out, err := t.Call(ctx, arguments)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return out, err
		}
	}
	err := fmt.Errorf("tool: %q not registered", name)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return "", err
}

// defaultSafetyClass maps a tool name to its built-in default safety
// classification, for the ListTools wire metadata. A future milestone
// may let users override per-tool via config.
//
// NOTE: chat/policy.go's safetyClassFor encodes the same name→class
// mapping for approval GATING. They're deliberately separate (wire
// metadata vs internal gate, different enum types, may diverge) — but
// keep the shared rows in sync when adding a built-in tool.
func defaultSafetyClass(name string) SafetyClass {
	switch name {
	case "read", "glob", "grep":
		return SafetyClassSafe
	case "write", "edit":
		return SafetyClassWrite
	case "bash":
		return SafetyClassExec
	default:
		// Unknown tool — treat as Exec until proven otherwise.
		return SafetyClassExec
	}
}
