package json

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// StringDefSchemaOf generates a JSON schema definition string for a given value.
// It returns the first definition string from the list of definitions.
func StringDefSchemaOf(v any) string {
	defs := StringDefsSchemaOf(v)
	if len(defs) == 0 {
		return ""
	}
	return defs[0]
}

// StringDefsSchemaOf generates a list of JSON schema definition strings for a given value.
// It extracts the "$defs" from the schema and returns them as a list of strings.
func StringDefsSchemaOf(v any) []string {
	schema := MapSchemaOf(v)
	defs := schema["$defs"]
	rv := make([]string, 0)
	m, ok := defs.(map[string]any)
	if !ok {
		return rv
	}
	for _, val := range m {
		marshal, _ := json.Marshal(val)
		rv = append(rv, string(marshal))
	}
	return rv
}

// StringSchemaOf generates a JSON schema string for a given value.
// It uses the jsonschema package to reflect the schema and returns it as a string.
func StringSchemaOf(v any) string {
	schema := jsonschema.Reflect(v)
	marshal, _ := json.Marshal(schema)
	return string(marshal)
}

// MapSchemaOf generates a JSON schema map for a given value.
// It uses the jsonschema package to reflect the schema and returns it as a map.
func MapSchemaOf(v any) map[string]any {
	schema := jsonschema.Reflect(v)
	marshal, _ := json.Marshal(schema)
	rv := make(map[string]any)
	_ = json.Unmarshal(marshal, &rv)
	return rv
}
