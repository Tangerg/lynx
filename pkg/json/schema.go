package json

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// StringSchemaOf takes any Go value and returns its JSON schema as a string.
// It uses the jsonschema package to reflect the structure of the value and
// then marshals the schema into a JSON string.
func StringSchemaOf(v any) string {
	schema := jsonschema.Reflect(v)
	marshal, _ := json.Marshal(schema)
	return string(marshal)
}

// MapSchemaOf takes any Go value and returns its JSON schema as a map.
// It uses the jsonschema package to reflect the structure of the value and
// then marshals the schema into a JSON byte slice, which is then unmarshaled
// into a map for easier manipulation and access.
func MapSchemaOf(v any) map[string]any {
	schema := jsonschema.Reflect(v)
	marshal, _ := json.Marshal(schema)
	rv := make(map[string]any)
	_ = json.Unmarshal(marshal, &rv)
	return rv
}
