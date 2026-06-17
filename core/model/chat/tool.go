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

// ToolConcurrency classifies how a tool call may run relative to the OTHER
// calls the model emitted in the same assistant message. A loop driver (the
// bundled [core/model/chat/middleware/tool]) uses it to run non-conflicting
// calls concurrently while keeping conflicting / opaque ones serial.
type ToolConcurrency int8

const (
	// ToolConcurrencyExclusive (the zero value, the safe default) runs the call
	// ALONE — never overlapping another call in the round. It is the right
	// class for opaque side effects (a shell command), shared mutable state,
	// and human-in-the-loop tools that pause the round. Until a tool opts into
	// another class every call is exclusive, so the loop stays fully serial.
	ToolConcurrencyExclusive ToolConcurrency = iota

	// ToolConcurrencyParallel runs the call concurrently with ANY other
	// non-exclusive call — it has no resource conflict (a pure read, a network
	// fetch, an isolated sub-agent). Reads, greps, code-intelligence queries,
	// and delegated sub-agents are parallel.
	ToolConcurrencyParallel

	// ToolConcurrencyKeyed runs the call concurrently with others EXCEPT those
	// touching the same resource: two keyed calls conflict (and serialize) only
	// when their [ConcurrentTool.ConcurrencyKey] matches. Different keys run in
	// parallel. File-mutating tools are keyed on the path they write, so edits
	// to distinct files parallelize while edits to the same file serialize.
	ToolConcurrencyKeyed
)

// ToolMetadata controls how the framework treats a tool's result after
// execution.
type ToolMetadata struct {
	// ReturnDirect routes the tool result straight back to the caller
	// without re-prompting the LLM. Useful for UI affordances and
	// notifications. False (the default) sends the result back to the
	// LLM for integration into the next reply.
	ReturnDirect bool

	// Concurrency classifies whether this tool's calls may overlap other calls
	// in the same round (see [ToolConcurrency]). The zero value
	// ([ToolConcurrencyExclusive]) keeps a tool serial — the conservative
	// default, so adding the field changes no existing tool's behavior until it
	// opts in.
	Concurrency ToolConcurrency
}

// ConcurrentTool is the optional capability a [ToolConcurrencyKeyed] tool
// implements to declare, per call, the resource its arguments touch. The loop
// driver serializes two keyed calls only when their keys match (e.g. two edits
// to the same file path), running different-key calls in parallel. A tool whose
// [ToolMetadata.Concurrency] is not Keyed need not implement this.
type ConcurrentTool interface {
	Tool

	// ConcurrencyKey returns the resource key the call described by arguments
	// touches — the file path for a file-mutating tool. An empty key means "no
	// known conflict" (treated as parallel for this call).
	ConcurrencyKey(arguments string) string
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

	// Metadata returns the post-execution behavior (return-direct, ...).
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
