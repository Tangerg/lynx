package tool

import (
	"encoding/json"
	"fmt"

	corejsonschema "github.com/Tangerg/scope/core/jsonschema"
)

// Schema derives a strict JSON Schema Draft 2020-12 contract for T. It follows
// encoding/json field names and options and the upstream invopop/jsonschema tag
// dialect. Structs reject additional properties by default.
func Schema[T any]() (json.RawMessage, error) {
	schema, err := corejsonschema.For[T]()
	if err != nil {
		return nil, fmt.Errorf("tool: derive schema: %w", err)
	}
	return schema.JSON(), nil
}
