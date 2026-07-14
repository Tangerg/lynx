package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFlattenContent_Empty(t *testing.T) {
	out, err := flattenContent(nil)
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestFlattenContent_SingleText(t *testing.T) {
	out, err := flattenContent([]sdkmcp.Content{&sdkmcp.TextContent{Text: "hi"}})
	require.NoError(t, err)
	assert.Equal(t, "hi", out)
}

func TestFlattenContent_MultipleSerialized(t *testing.T) {
	in := []sdkmcp.Content{
		&sdkmcp.TextContent{Text: "a"},
		&sdkmcp.TextContent{Text: "b"},
	}
	out, err := flattenContent(in)
	require.NoError(t, err)

	// Must be JSON-decodable as an array of objects with the type discriminator.
	var arr []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &arr))
	require.Len(t, arr, 2)
	assert.Equal(t, "text", arr[0]["type"])
	assert.Equal(t, "a", arr[0]["text"])
	assert.Equal(t, "b", arr[1]["text"])
}

func TestFlattenContent_NonTextSingle(t *testing.T) {
	// A single non-Text element must still be serialized with type info.
	in := []sdkmcp.Content{&sdkmcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}}
	out, err := flattenContent(in)
	require.NoError(t, err)

	var arr []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &arr))
	require.Len(t, arr, 1)
	assert.Equal(t, "image", arr[0]["type"])
}

func TestFirstTextOrFallback_PrefersFirstNonEmpty(t *testing.T) {
	got := firstTextOrFallback([]sdkmcp.Content{
		&sdkmcp.TextContent{Text: ""},
		&sdkmcp.TextContent{Text: "real"},
	}, "fallback")
	assert.Equal(t, "real", got)
}

func TestFirstTextOrFallback_UsesFallbackWhenNoText(t *testing.T) {
	got := firstTextOrFallback(nil, "fallback message")
	assert.Equal(t, "fallback message", got)
}

func TestSchemaToJSON_Variants(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, string(emptyObjectSchema)},
		{"empty string", "", string(emptyObjectSchema)},
		{"string passthrough", `{"type":"object","x":1}`, `{"type":"object","x":1}`},
		{"raw message", json.RawMessage(`{"type":"object"}`), `{"type":"object"}`},
		{"empty raw", json.RawMessage(``), string(emptyObjectSchema)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schemaToJSON(tc.in)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestSchemaToJSON_StructSerializes(t *testing.T) {
	in := map[string]any{"type": "object", "additionalProperties": false}
	got, err := schemaToJSON(in)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &decoded))
	assert.Equal(t, "object", decoded["type"])
	assert.Equal(t, false, decoded["additionalProperties"])
}
