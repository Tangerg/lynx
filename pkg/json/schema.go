package json

import (
	"github.com/invopop/jsonschema"
)

// StringDefSchemaOf generates a JSON schema definition string for a given value.
// It returns the first definition string from the list of definitions.
func StringDefSchemaOf(v any) string {
	schema := jsonschema.Reflect(v)
	for _, def := range schema.Definitions {
		json, _ := def.MarshalJSON()
		return string(json)
	}
	return ""
}
