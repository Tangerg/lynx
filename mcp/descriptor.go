package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	corechat "github.com/Tangerg/lynx/core/chat"
)

const emptyObjectSchema = `{"type":"object","additionalProperties":false}`

var errNilDescriptor = errors.New("mcp: descriptor must not be nil")

type descriptorSnapshot struct {
	value sdkmcp.Tool
}

func newDescriptorSnapshot(descriptor *sdkmcp.Tool) (descriptorSnapshot, error) {
	if descriptor == nil {
		return descriptorSnapshot{}, errNilDescriptor
	}
	if descriptor.Name == "" {
		return descriptorSnapshot{}, errors.New("mcp: descriptor name must not be empty")
	}

	data, err := json.Marshal(descriptor)
	if err != nil {
		return descriptorSnapshot{}, err
	}
	var snapshot sdkmcp.Tool
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return descriptorSnapshot{}, err
	}
	return descriptorSnapshot{value: snapshot}, nil
}

func (d descriptorSnapshot) name() string {
	return d.value.Name
}

func (d descriptorSnapshot) definition(publicName string) (corechat.ToolDefinition, error) {
	schema, err := d.inputSchema()
	if err != nil {
		return corechat.ToolDefinition{}, err
	}
	definition := corechat.ToolDefinition{
		Name:        publicName,
		Description: d.value.Description,
		InputSchema: schema,
	}
	if err := definition.Validate(); err != nil {
		return corechat.ToolDefinition{}, err
	}
	return definition, nil
}

func (d descriptorSnapshot) annotations() sdkmcp.ToolAnnotations {
	if d.value.Annotations == nil {
		return sdkmcp.ToolAnnotations{}
	}
	annotations := *d.value.Annotations
	if annotations.DestructiveHint != nil {
		annotations.DestructiveHint = new(*annotations.DestructiveHint)
	}
	if annotations.OpenWorldHint != nil {
		annotations.OpenWorldHint = new(*annotations.OpenWorldHint)
	}
	return annotations
}

func (d descriptorSnapshot) inputSchema() (json.RawMessage, error) {
	switch value := d.value.InputSchema.(type) {
	case nil:
		return json.RawMessage(emptyObjectSchema), nil
	case string:
		if value == "" {
			return json.RawMessage(emptyObjectSchema), nil
		}
		return json.RawMessage(value), nil
	case json.RawMessage:
		if len(value) == 0 {
			return json.RawMessage(emptyObjectSchema), nil
		}
		return bytes.Clone(value), nil
	case []byte:
		if len(value) == 0 {
			return json.RawMessage(emptyObjectSchema), nil
		}
		return bytes.Clone(value), nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode tool input schema: %w", err)
		}
		return encoded, nil
	}
}
