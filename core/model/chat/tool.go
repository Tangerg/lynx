package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
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
	// Required by NewTool so the LLM can format arguments correctly.
	// Leave it empty when using NewJSONTool; the schema is derived from
	// the generic input type.
	InputSchema string
}

// Tool is the executable contract every tool exposes — describable to
// the LLM ([Tool.Definition]) and runnable by the framework ([Tool.Call]).
//
// Tools that cannot run in-process — human approval gates, frontend
// delegation, async dispatch — are not modeled as a separate type.
// Instead, higher layers (tool middleware, decorators) wrap a Tool
// and surface control-flow signals via sentinel errors. See
// production examples in the application runtime for HITL and policy-gated
// tooling.
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

func validateToolDefinition(caller string, definition ToolDefinition, requireSchema bool) error {
	if definition.Name == "" {
		return fmt.Errorf("%s: definition.Name must not be empty", caller)
	}
	if requireSchema && definition.InputSchema == "" {
		return fmt.Errorf("%s: definition.InputSchema must not be empty", caller)
	}
	return nil
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
	if err := validateToolDefinition("chat.NewTool", definition, true); err != nil {
		return nil, err
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewTool: execFunc must not be nil")
	}

	return &tool{
		definition: definition,
		execFunc:   execFunc,
	}, nil
}

type jsonTool[In any] struct {
	definition ToolDefinition
	execFunc   func(ctx context.Context, in In) (string, error)
}

func (t *jsonTool[In]) Definition() ToolDefinition { return t.definition }

func (t *jsonTool[In]) Call(ctx context.Context, arguments string) (string, error) {
	var in In
	if err := json.Unmarshal([]byte(arguments), &in); err != nil {
		return "", fmt.Errorf("chat.NewJSONTool: decode arguments: %w", err)
	}
	return t.execFunc(ctx, in)
}

// NewJSONTool builds a [Tool] whose argument schema and decoder both come from
// In. It is the typed counterpart to [NewTool]: callers provide only the tool
// name/description and the executable behavior; the InputSchema field must be
// left empty and is derived from In.
func NewJSONTool[In any](
	definition ToolDefinition,
	execFunc func(ctx context.Context, in In) (string, error),
) (Tool, error) {
	if err := validateToolDefinition("chat.NewJSONTool", definition, false); err != nil {
		return nil, err
	}
	if definition.InputSchema != "" {
		return nil, errors.New("chat.NewJSONTool: definition.InputSchema must be empty")
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewJSONTool: execFunc must not be nil")
	}

	var zero In
	schema, err := pkgjson.StringDefSchemaOf(zero)
	if err != nil {
		return nil, fmt.Errorf("chat.NewJSONTool: derive input schema: %w", err)
	}
	definition.InputSchema = schema

	return &jsonTool[In]{
		definition: definition,
		execFunc:   execFunc,
	}, nil
}
