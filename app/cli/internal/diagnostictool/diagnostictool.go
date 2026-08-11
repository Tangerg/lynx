// Package diagnostictool defines read-only runtime diagnostics that the CLI can
// invoke outside an agent run. JSON stays opaque inside the domain so runtime
// protocol maps cannot leak past the adapter boundary.
package diagnostictool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Safety string

const (
	Safe Safety = "safe"
)

func (safety Safety) Validate() error {
	if safety != Safe {
		return fmt.Errorf("direct diagnostic tool safety must be safe, got %q", safety)
	}
	return nil
}

type Descriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Safety      Safety
}

func (descriptor Descriptor) Validate() error {
	if strings.TrimSpace(descriptor.Name) == "" {
		return errors.New("diagnostic tool name is empty")
	}
	if err := descriptor.Safety.Validate(); err != nil {
		return fmt.Errorf("diagnostic tool %s: %w", descriptor.Name, err)
	}
	return validateObject("diagnostic tool schema", descriptor.Schema)
}

func (descriptor Descriptor) Clone() Descriptor {
	descriptor.Schema = append(json.RawMessage(nil), descriptor.Schema...)
	return descriptor
}

type Invocation struct {
	Tool      Descriptor
	Arguments json.RawMessage
	Workspace string
}

func (invocation Invocation) Validate() error {
	if err := invocation.Tool.Validate(); err != nil {
		return fmt.Errorf("diagnostic tool invocation: %w", err)
	}
	if strings.TrimSpace(invocation.Workspace) == "" {
		return errors.New("diagnostic tool invocation workspace is empty")
	}
	return validateObject("diagnostic tool arguments", invocation.Arguments)
}

type Result struct{ JSON json.RawMessage }

func (result Result) Validate() error {
	if len(result.JSON) == 0 || !json.Valid(result.JSON) {
		return errors.New("diagnostic tool result is not valid JSON")
	}
	return nil
}

func (result Result) Clone() Result {
	return Result{JSON: append(json.RawMessage(nil), result.JSON...)}
}

type Service interface {
	Tools(context.Context) ([]Descriptor, error)
	Invoke(context.Context, Invocation) (Result, error)
}

// ParseArguments owns the direct-invocation JSON-object invariant without
// requiring delivery code to manufacture a partial tool descriptor merely to
// validate user input.
func ParseArguments(value string) (json.RawMessage, error) {
	arguments := json.RawMessage(strings.TrimSpace(value))
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if err := validateObject("diagnostic tool arguments", arguments); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), arguments...), nil
}

func validateObject(name string, value json.RawMessage) error {
	if len(value) == 0 || !json.Valid(value) {
		return fmt.Errorf("%s is not valid JSON", name)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return fmt.Errorf("%s must be a JSON object", name)
	}
	return nil
}
