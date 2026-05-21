package engine

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// ToolObserver receives tool-call lifecycle notifications. Each pair
// of OnToolCallStart / OnToolCallEnd is matched by an opaque CallID
// so observers correlating the two halves don't have to keep their
// own tracking map.
//
// Implementations must be safe for concurrent calls — a chat turn
// may dispatch multiple tools simultaneously when the model emits
// parallel tool_calls.
type ToolObserver interface {
	OnToolCallStart(callID, toolName, arguments string)
	OnToolCallEnd(callID, toolName, output string, err error)
}

// toolObserverDecorator is the process-scope [core.ToolDecorator]
// the engine attaches when [RunChat] is called with an observer.
// It wraps every resolved [core.AgentTool] with [observedTool] so
// invocations land on the observer without changing the underlying
// tool implementation.
type toolObserverDecorator struct {
	observer ToolObserver
}

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with platform
// scope, and this decorator is process-scoped.
func (d *toolObserverDecorator) Name() string { return "lyra-tool-observer" }

// DecorateTool wraps tool with [observedTool], threading the
// observer into every Call so start / end notifications fire.
// Action is intentionally ignored — Lyra emits per-tool, not
// per-action, events.
func (d *toolObserverDecorator) DecorateTool(_ core.Process, _ core.Action, tool core.AgentTool) core.AgentTool {
	return &observedTool{inner: tool, observer: d.observer}
}

// observedTool is the per-call wrapper. CallID is generated fresh
// per invocation so two concurrent calls to the same tool stay
// distinguishable on the observer side.
type observedTool struct {
	inner    chat.Tool
	observer ToolObserver
}

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }
func (o *observedTool) Metadata() chat.ToolMetadata     { return o.inner.Metadata() }

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	callID := uuid.NewString()
	name := o.inner.Definition().Name

	o.observer.OnToolCallStart(callID, name, arguments)
	output, err := o.inner.Call(ctx, arguments)
	o.observer.OnToolCallEnd(callID, name, output, err)

	return output, err
}
