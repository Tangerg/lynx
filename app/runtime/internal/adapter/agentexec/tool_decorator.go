package agentexec

import (
	"context"
	"crypto/rand"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type mcpToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// toolObserverMiddleware is the process-scope [core.ToolMiddleware]
// the engine attaches when [StartTurn] is called with an observer.
// It wraps every resolved [core.AgentTool] with [observedTool] so
// invocations land on the observer without changing the underlying
// tool implementation.
type toolObserverMiddleware struct {
	observation *toolObservation
}

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with engine
// scope, and this decorator is process-scoped.
func (d *toolObserverMiddleware) Name() string { return "tool-observer" }

// WrapTool wraps tool with [observedTool], threading the
// observer into every Call so start / end notifications fire.
// Action is intentionally ignored — Lyra emits per-tool, not
// per-action, events.
func (d *toolObserverMiddleware) WrapTool(_ core.ProcessView, _ core.Action, tool tools.Tool) tools.Tool {
	return &observedTool{inner: tool, observation: d.observation}
}

// observedTool is the per-call wrapper. Managed calls reuse the coordinator's
// process+round+model identity; calls made outside ToolLoop receive a fresh ID.
type observedTool struct {
	inner       tools.Tool
	observation *toolObservation
}

var _ tools.FileMutationReporter = (*observedTool)(nil)

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }

// ReturnsDirect forwards the wrapped tool's return-direct declaration. This
// decorator wraps every resolved tool, so dropping the marker would turn
// return-direct tools into regular continuation tools.
func (o *observedTool) ReturnsDirect() bool {
	if direct, ok := o.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

// ConcurrencyKey preserves the wrapped tool's optional scheduling contract.
// Observation is per-call and concurrency-safe; wrapping a read-only or
// resource-keyed tool must not silently downgrade it to exclusive execution.
func (o *observedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if capability, ok := o.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return capability.ConcurrencyKey(arguments)
	}
	return "", false
}

// MutationPaths keeps observation transparent to file-aware outer middleware.
// Lifecycle reporting itself consumes the same method after a successful call.
func (o *observedTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := o.inner.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	name := o.inner.Definition().Name
	call, bound := o.observation.invocation(ctx, name, arguments)
	if !bound {
		call = &observedModelCall{id: "direct:" + rand.Text(), process: processRefFromContext(ctx), name: name, arguments: arguments}
	}

	mutations, _ := o.inner.(tools.FileMutationReporter)
	target := ToolApprovalTarget{FileMutations: mutations}
	if identity, ok := o.inner.(mcpToolIdentity); ok {
		server, remote := identity.MCPToolIdentity()
		if server != "" && remote != "" {
			target.MCP = mcpserver.ToolRef{Server: server, Tool: remote}
		}
	}
	v := o.observation.target.ApproveToolCall(ctx, call.id, name, arguments, target)
	if v.Arguments != "" {
		arguments = v.Arguments
	}
	if bound {
		o.observation.prepare(call, arguments)
	} else {
		o.observation.target.OnToolCallStart(call.process, call.id, name, arguments)
	}
	switch {
	case v.Interrupt != nil:
		return "", v.Interrupt
	case v.Denied:
		// Recoverable denial: the model sees DenyReason as the tool
		// result and adapts instead of aborting. Start/End still fire so
		// UI counts stay matched; End carries ErrToolDenied so the wire
		// renders a distinct "denied" terminal (not a green success).
		o.observation.finish(call, bound, arguments, v.DenyReason, nil, nil, ErrToolDenied)
		return v.DenyReason, nil
	}

	output, err := o.inner.Call(ctx, arguments)
	displayed := output
	var ref *offload.Ref
	if err == nil {
		// Evict an oversized body to the blob store, substituting a bounded preview
		// for BOTH the transcript (finish → OnToolCallEnd) and the model (the
		// returned value the tool loop records) — so the full body lives in the
		// blob alone. The transcript store rehydrates it through the typed item-to-
		// blob relationship when serving items to the UI.
		displayed, ref = o.observation.evict(ctx, name, output)
	}
	o.observation.finish(call, bound, arguments, displayed, ref, o.successfulMutationPaths(arguments, err), err)

	return displayed, err
}

func (o *observedTool) successfulMutationPaths(arguments string, callErr error) []string {
	if callErr != nil {
		return nil
	}
	paths, err := o.MutationPaths(arguments)
	if err != nil {
		return nil
	}
	paths = slices.Clone(paths)
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
