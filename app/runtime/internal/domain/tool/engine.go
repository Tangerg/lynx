package tool

import (
	"context"
	"errors"
	"fmt"

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

// source is the narrow surface the registry consumes: just a
// snapshot of the currently-registered chat tools. *kernel.Engine
// satisfies it implicitly via its Tools() accessor; tests pass a
// stub that returns a fixed slice without needing a real platform.
type source interface {
	Tools() []chat.Tool
}

// New returns the [Registry] implementation backed by src. List
// snapshots the registered tools; Invoke routes by tool name to
// the registered tool's Call method (no agent loop involved —
// direct synchronous invocation).
func New(src source) (Registry, error) {
	if src == nil {
		return nil, errors.New("tool: source is required")
	}
	return &registry{src: src}, nil
}

// registry is the engine-backed registered-tool directory. The source is
// typically the engine but can be any tool snapshot provider in tests.
type registry struct {
	src source
}

func (r *registry) List(_ context.Context) ([]Tool, error) {
	chatTools := r.src.Tools()
	out := make([]Tool, 0, len(chatTools))
	for _, t := range chatTools {
		def := t.Definition()
		out = append(out, Tool{
			Name:        def.Name,
			Description: def.Description,
			Schema:      def.InputSchema,
			SafetyClass: SafetyClassFor(def.Name),
		})
	}
	return out, nil
}

func (r *registry) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("tool: name must not be empty")
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
	err := fmt.Errorf("tool: %q not registered", name)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return "", err
}

// SafetyClassFor maps a built-in tool name to its side-effect safety class. It
// is the single source of truth for the name→class mapping — consumed here for
// the tools.list wire metadata AND by the approval gate ([approval.GateFor]) —
// so the two views never drift apart. Unknown tools (task, MCP, third-party
// tools) fall to Exec (fail-conservative: they may do anything). A future
// milestone may let users override per-tool via config.
func SafetyClassFor(name string) SafetyClass {
	switch name {
	case "read", "glob", "grep", "lsp", "lsp_diagnostics", "skill", "ask_user", "exit_plan_mode", "sourcegraph_search", "schedule_list":
		// lsp / lsp_diagnostics are read-only code-intelligence queries — same
		// class as read/glob/grep. skill only reads skill files. ask_user has no
		// side effect (it IS a HITL interrupt, so gating it would double-prompt).
		// exit_plan_mode is the way out of the read-only plan stance — it must
		// stay Safe or the agent would be trapped in plan mode.
		return SafetyClassSafe
	case "write", "edit", "multiedit", "apply_patch", "download", "schedule_create", "schedule_update", "schedule_delete":
		return SafetyClassWrite
	default:
		return SafetyClassExec
	}
}

// ClassName is the wire vocabulary (API.md §4.4 SafetyClass: "safe" | "write" |
// "exec") for a class — the canonical string a client renders, stamped on the
// live toolCall Item and the approval prompt.
func ClassName(c SafetyClass) string {
	switch c {
	case SafetyClassSafe:
		return "safe"
	case SafetyClassWrite:
		return "write"
	default:
		return "exec"
	}
}
