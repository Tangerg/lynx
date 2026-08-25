package mcp

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescriptorInputSchema(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", want: emptyObjectSchema},
		{name: "empty string", value: "", want: emptyObjectSchema},
		{name: "string", value: `{"type":"object","x":1}`, want: `{"type":"object","x":1}`},
		{name: "raw message", value: json.RawMessage(`{"type":"object"}`), want: `{"type":"object"}`},
		{name: "empty raw message", value: json.RawMessage{}, want: emptyObjectSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := descriptorSnapshot{value: sdkmcp.Tool{InputSchema: test.value}}
			got, err := descriptor.inputSchema()
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(got))
		})
	}
}

func TestDescriptorInputSchemaMarshalsSDKValue(t *testing.T) {
	descriptor := descriptorSnapshot{value: sdkmcp.Tool{InputSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}}}
	got, err := descriptor.inputSchema()
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"object","additionalProperties":false}`, string(got))
}

func TestDescriptorAnnotationsAreIsolated(t *testing.T) {
	descriptor := descriptorSnapshot{value: sdkmcp.Tool{Annotations: &sdkmcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(false),
	}}}

	first := descriptor.annotations()
	*first.DestructiveHint = true
	*first.OpenWorldHint = true

	second := descriptor.annotations()
	assert.False(t, *second.DestructiveHint)
	assert.False(t, *second.OpenWorldHint)
}

func TestDescriptorSnapshotOwnsSDKValue(t *testing.T) {
	destructive := false
	original := &sdkmcp.Tool{
		Name:        "remote",
		Description: "original",
		InputSchema: map[string]any{"type": "object"},
		Annotations: &sdkmcp.ToolAnnotations{DestructiveHint: &destructive},
	}
	snapshot, err := newDescriptorSnapshot(original)
	require.NoError(t, err)

	original.Name = "mutated"
	original.Description = "mutated"
	original.InputSchema.(map[string]any)["type"] = "array"
	*original.Annotations.DestructiveHint = true

	definition, err := snapshot.definition("public")
	require.NoError(t, err)
	assert.Equal(t, "remote", snapshot.name())
	assert.Equal(t, "original", definition.Description)
	assert.JSONEq(t, `{"type":"object"}`, string(definition.InputSchema))
	assert.False(t, *snapshot.annotations().DestructiveHint)
}
