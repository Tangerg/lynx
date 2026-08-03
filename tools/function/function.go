// Package function derives JSON Schema from typed Go inputs and adapts typed
// functions to model-facing tools. Use [tool.NewFunc] when the definition is
// already explicit and schema derivation is not wanted.
package function

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
	toolschema "github.com/Tangerg/lynx/tools/internal/schema"
)

// Config describes a typed function tool. New derives its input schema from
// In so the decoder and model-visible contract cannot drift independently.
type Config struct {
	Name        string
	Description string
}

// New builds a typed function tool whose input schema is derived from In.
func New[In, Out any](config Config, function func(context.Context, In) (Out, error)) (*tool.Func[In, Out], error) {
	definition := chat.ToolDefinition{
		Name:        config.Name,
		Description: config.Description,
		InputSchema: json.RawMessage(`{}`),
	}
	// Validate the explicit contract and typed adapter before asking the schema
	// library to inspect In. This keeps construction errors owned by tool and
	// preserves ErrInvalidTool identity for every invalid function signature.
	if _, err := tool.NewFunc(definition, function); err != nil {
		return nil, err
	}

	var input In
	schema, err := toolschema.String(input)
	if err != nil {
		return nil, fmt.Errorf("function: derive input schema: %w", err)
	}
	definition.InputSchema = json.RawMessage(schema)
	return tool.NewFunc(definition, function)
}
