package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// mathArgs is the input shape for every arithmetic tool: two operands.
type mathArgs struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// MathTools returns the arithmetic tool set — add, subtract, multiply, and
// divide — each taking {"a", "b"} and returning the numeric result as a
// string. Division by zero is reported as a tool error (fed back to the
// model when the loop is configured for it).
func MathTools() []chat.Tool {
	schema := pkgjson.MustStringDefSchemaOf(mathArgs{})

	mk := func(name, description string, op func(a, b float64) (float64, error)) chat.Tool {
		t, _ := chat.NewTool(
			chat.ToolDefinition{Name: name, Description: description, InputSchema: schema},
			chat.ToolMetadata{},
			func(_ context.Context, arguments string) (string, error) {
				var in mathArgs
				if err := json.Unmarshal([]byte(arguments), &in); err != nil {
					return "", fmt.Errorf("%s: parse arguments: %w", name, err)
				}
				res, err := op(in.A, in.B)
				if err != nil {
					return "", fmt.Errorf("%s: %w", name, err)
				}
				return strconv.FormatFloat(res, 'g', -1, 64), nil
			},
		)
		// Construction inputs are compile-time constants (non-empty name +
		// schema, non-nil func), so NewTool cannot error here.
		return t
	}

	return []chat.Tool{
		mk("add", "Add two numbers and return a + b.", func(a, b float64) (float64, error) { return a + b, nil }),
		mk("subtract", "Subtract b from a and return a - b.", func(a, b float64) (float64, error) { return a - b, nil }),
		mk("multiply", "Multiply two numbers and return a * b.", func(a, b float64) (float64, error) { return a * b, nil }),
		mk("divide", "Divide a by b and return a / b.", func(a, b float64) (float64, error) {
			if b == 0 {
				return 0, errors.New("division by zero")
			}
			return a / b, nil
		}),
	}
}
