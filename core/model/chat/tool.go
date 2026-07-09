package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cast"

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

	// InputSchema is the JSON Schema describing the argument shape the LLM
	// formats its call against. Leave it empty: [NewTool] derives it from the
	// generic input type In.
	InputSchema string
}

// defaultToolSuccess is what the model sees when a tool succeeds but returns an
// empty result — an affirmative outcome instead of a blank tool message.
const defaultToolSuccess = "(tool call succeeded with no output)"

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
type tool[In, Out any] struct {
	definition ToolDefinition
	execFunc   func(ctx context.Context, in In) (Out, error)
}

func (t *tool[In, Out]) Definition() ToolDefinition { return t.definition }

func (t *tool[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	var in In
	// The tool-call contract allows omitted arguments when every field is
	// optional: some providers emit "" for a parameterless call where others
	// emit "{}". Treat empty as the empty object so In decodes to its zero value
	// instead of failing on "unexpected end of JSON input".
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	if err := json.Unmarshal([]byte(arguments), &in); err != nil {
		return "", fmt.Errorf("chat.NewTool: decode arguments: %w", err)
	}
	out, err := t.execFunc(ctx, in)
	if err != nil {
		return "", err
	}
	result, err := stringifyToolResult(out)
	if err != nil {
		return "", fmt.Errorf("chat.NewTool: encode result: %w", err)
	}
	if result == "" {
		return defaultToolSuccess, nil
	}
	return result, nil
}

// stringifyToolResult renders a tool's typed output as the string the model
// sees. Scalars and fmt.Stringer values go through cast so a string (or []byte,
// number, bool, Stringer) renders VERBATIM — unquoted, unlike json.Marshal;
// composite results (structs, maps, slices) that cast can't render fall back to
// JSON encoding. Order matters: cast first keeps a string result unquoted.
func stringifyToolResult[Out any](out Out) (string, error) {
	if s, err := cast.ToStringE(out); err == nil {
		return s, nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateToolDefinition(caller string, definition ToolDefinition) error {
	if definition.Name == "" {
		return fmt.Errorf("%s: definition.Name must not be empty", caller)
	}
	return nil
}

// NewTool builds a [Tool] from a typed function. The argument schema and decoder
// come from In (a Go struct whose json / jsonschema tags describe the LLM-facing
// parameters); the return value Out is rendered to the string the model sees —
// a string verbatim, anything else JSON-encoded, and an empty result replaced
// with a default success message. definition.Name is required, definition.
// InputSchema must be empty (it is derived from In), and execFunc must be
// non-nil. Use In = struct{} for a parameterless tool.
//
// To gate execution on human approval or delegate it elsewhere, have the tool
// (or a decorator) return the control-flow error your loop driver understands;
// the chat layer always treats a registered tool as runnable.
//
// Example:
//
//	tool, err := chat.NewTool[addInput, int](
//	    chat.ToolDefinition{Name: "add", Description: "add two numbers"},
//	    func(ctx context.Context, in addInput) (int, error) { return in.A + in.B, nil },
//	)
func NewTool[In, Out any](
	definition ToolDefinition,
	execFunc func(ctx context.Context, in In) (Out, error),
) (Tool, error) {
	if err := validateToolDefinition("chat.NewTool", definition); err != nil {
		return nil, err
	}
	if definition.InputSchema != "" {
		return nil, errors.New("chat.NewTool: definition.InputSchema must be empty (it is derived from In)")
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewTool: execFunc must not be nil")
	}

	var zero In
	schema, err := pkgjson.StringDefSchemaOf(zero)
	if err != nil {
		return nil, fmt.Errorf("chat.NewTool: derive input schema: %w", err)
	}
	definition.InputSchema = schema

	return &tool[In, Out]{definition: definition, execFunc: execFunc}, nil
}
