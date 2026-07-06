package chat

import (
	"context"
	"errors"
)

// ToolDefinition is the static description of a tool that LLMs see when
// deciding whether and how to call it. The InputSchema is a JSON Schema
// the model uses to format its arguments.
type ToolDefinition struct {
	// Name uniquely identifies the tool. Required.
	Name string

	// Description is a human-readable hint shown to the LLM.
	Description string

	// InputSchema is a JSON Schema describing the argument shape.
	// Required so the LLM can format arguments correctly.
	InputSchema string
}

// Tool is the executable contract every tool exposes — describable to
// the LLM ([Tool.Definition]) and runnable by the framework ([Tool.Call]).
//
// Tools that cannot run in-process — human approval gates, frontend
// delegation, async dispatch — are not modeled as a separate type.
// Instead, layers above (agent middleware, tool decorators) wrap a Tool
// and surface control-flow signals via sentinel errors. See
// agent/hitl and agent/toolpolicy for production examples.
type Tool interface {
	// Definition returns the static description shown to the LLM.
	Definition() ToolDefinition

	// Call runs the tool's body. arguments is the JSON-encoded payload the
	// LLM produced. The string result is the tool's output; the driver decides
	// whether to feed it back to the LLM or return it directly to the caller.
	//
	// A non-nil error means the tool could not produce a result. What happens
	// to it is decided by whatever DRIVES the call, not by this interface: a
	// loop driver feeds an ordinary error back to the model as a result so it can
	// recover, and STOPS the loop only on its own control-flow signals (for
	// example context cancellation/deadline or driver-specific halt errors). A tool
	// author who wants control over the wording can fold an operational failure
	// (file not found, wrong credentials, a non-zero exit, an HTTP 4xx) into the
	// result string instead of returning an error; both reach the model.
	//
	// On a HITL interrupt the loop does NOT re-run the turn: it surfaces the
	// in-flight call to the caller and, on resume, continues AT this call (the
	// approved call executes exactly once). So Call is invoked once per logical
	// step, the same as a normal round.
	Call(ctx context.Context, arguments string) (string, error)
}

// tool is the concrete backing for tools built via [NewTool].
type tool struct {
	definition ToolDefinition
	execFunc   func(ctx context.Context, arguments string) (string, error)
}

func (t *tool) Definition() ToolDefinition { return t.definition }

func (t *tool) Call(ctx context.Context, arguments string) (string, error) {
	return t.execFunc(ctx, arguments)
}

// NewTool builds a [Tool] backed by execFunc. All three components are
// required: an empty name, an empty input schema, or a nil exec function
// will return an error.
//
// To gate execution on human approval or to delegate execution to an
// external system, have the tool (or a decorator around it) return the
// control-flow error understood by your loop driver. The chat layer itself
// always treats a registered tool as runnable.
//
// Example:
//
//	tool, err := chat.NewTool(
//	    chat.ToolDefinition{Name: "add", InputSchema: addSchema},
//	    func(ctx context.Context, args string) (string, error) { ... },
//	)
func NewTool(definition ToolDefinition, execFunc func(ctx context.Context, arguments string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("chat.NewTool: definition.Name must not be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("chat.NewTool: definition.InputSchema must not be empty")
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewTool: execFunc must not be nil")
	}

	return &tool{
		definition: definition,
		execFunc:   execFunc,
	}, nil
}
