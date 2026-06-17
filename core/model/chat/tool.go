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

// ToolMetadata describes a tool's framework-facing behavior: the
// meta-parameters a tool-loop driver reads to decide how to treat the
// tool, independent of what the tool computes. It is a static descriptor
// returned by [Tool.Metadata] — NOT the tool's result.
type ToolMetadata struct {
	// ReturnDirect routes the tool result straight back to the caller
	// without re-prompting the LLM. Useful for UI affordances and
	// notifications. False (the default) sends the result back to the
	// LLM for integration into the next reply.
	ReturnDirect bool

	// Extra carries driver- or provider-specific meta-parameters that
	// don't warrant a typed field. Access via [ToolMetadata.Get] /
	// [ToolMetadata.Set].
	Extra map[string]any
}

// ensureExtra lazily allocates Extra. Used by [ToolMetadata.Set]
// only — Get must not mutate state.
func (m *ToolMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe
// to call concurrently with other Get calls; concurrent with Set is not.
func (m *ToolMetadata) Get(key string) (any, bool) {
	if m == nil || m.Extra == nil {
		return nil, false
	}
	value, exists := m.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (m *ToolMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Tool is the executable contract every tool exposes — describable to
// the LLM (Definition / Metadata) and runnable by the framework (Call).
//
// Tools that cannot run in-process — human approval gates, frontend
// delegation, async dispatch — are not modeled as a separate type.
// Instead, layers above (agent middleware, tool decorators) wrap a Tool
// and surface control-flow signals via sentinel errors. See
// agent/hitl and agent/toolpolicy for production examples.
type Tool interface {
	// Definition returns the static description shown to the LLM.
	Definition() ToolDefinition

	// Metadata returns the tool's framework-facing meta-parameters
	// (return-direct, ...).
	Metadata() ToolMetadata

	// Call runs the tool's body. arguments is the JSON-encoded payload the
	// LLM produced. The string result is the tool's output — fed back to the
	// LLM, or returned to the caller when ReturnDirect is true.
	//
	// A non-nil error means the tool could not produce a result. What happens
	// to it is decided by whatever DRIVES the call, not by this interface: a
	// loop driver (the bundled [core/model/chat/middleware/tool], or a
	// third-party one) feeds an ordinary error back to the model as a result so
	// it can recover, and STOPS the loop only on a control-flow signal — context
	// cancellation / deadline, or a [ToolHalt] (a fatal abort, or a HITL
	// interrupt the run resumes from). See [ToolHalt] and that package for the
	// exact contract. A tool author who wants control over the wording can fold
	// an operational failure (file not found, wrong credentials, a non-zero
	// exit, an HTTP 4xx) into the result string instead of returning an error;
	// both reach the model.
	//
	// On a HITL interrupt the loop does NOT re-run the turn: it surfaces the
	// in-flight call to the caller and, on resume, continues AT this call (the
	// approved call executes exactly once). So Call is invoked once per logical
	// step, the same as a normal round.
	Call(ctx context.Context, arguments string) (string, error)
}

// ToolHalt is the one control-flow contract a tool error can carry. When an
// error returned from [Tool.Call] implements it, the tool-calling loop STOPS
// rather than feeding the error back to the model as a recoverable result: the
// loop propagates the error unchanged so an outer layer can act on it. An
// ordinary error — one that does NOT implement ToolHalt — is recoverable (the
// loop wraps it as a tool result and the model adapts).
//
// It lives here, in the protocol package, on purpose: it is the contract
// between a [Tool] and ANY tool-loop driver (the bundled
// core/model/chat/middleware/tool, or a third-party one), and the guiding
// model for third-party tool middleware. Implement it on your own sentinel
// errors; agent/hitl.InterruptError is one example.
type ToolHalt interface {
	error

	// Abort reports the halt's intent:
	//   - true  → the run cannot continue (a fatal failure the model can't
	//     fix); the loop propagates it and the run fails.
	//   - false → the run is suspended for human input (HITL) and is expected
	//     to resume; the loop propagates it and an outer layer parks the run.
	// Either way the loop stops and propagates; the bool only tells the outer
	// layer (and the loop's HITL resume handling) which kind it is.
	Abort() bool
}

// tool is the concrete backing for tools built via [NewTool].
type tool struct {
	definition ToolDefinition
	metadata   ToolMetadata
	execFunc   func(ctx context.Context, arguments string) (string, error)
}

func (t *tool) Definition() ToolDefinition { return t.definition }
func (t *tool) Metadata() ToolMetadata     { return t.metadata }

func (t *tool) Call(ctx context.Context, arguments string) (string, error) {
	return t.execFunc(ctx, arguments)
}

// NewTool builds a [Tool] backed by execFunc. All three components are
// required: an empty name, an empty input schema, or a nil exec function
// will return an error.
//
// To gate execution on human approval or to delegate execution to an
// external system, have the tool (or a decorator around it) return a
// [ToolHalt] error — the loop propagates it instead of feeding it back, so an
// outer layer can park or fail the run. agent/hitl.InterruptError is the
// reference HITL implementation. The chat layer itself always treats a
// registered tool as runnable.
//
// Example:
//
//	tool, err := chat.NewTool(
//	    chat.ToolDefinition{Name: "add", InputSchema: addSchema},
//	    chat.ToolMetadata{},
//	    func(ctx context.Context, args string) (string, error) { ... },
//	)
func NewTool(definition ToolDefinition, metadata ToolMetadata, execFunc func(ctx context.Context, arguments string) (string, error)) (Tool, error) {
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
		metadata:   metadata,
		execFunc:   execFunc,
	}, nil
}
