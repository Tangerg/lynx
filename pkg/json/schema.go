package json

import (
	"reflect"

	"github.com/invopop/jsonschema"
)

// StringDefSchemaOf generates a JSON schema definition string for a given value.
// It returns the first definition string from the list of definitions.
func StringDefSchemaOf(v any) string {
	r := &jsonschema.Reflector{
		Anonymous:      true,
		ExpandedStruct: false,
		DoNotReference: true,
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Struct {
		r.ExpandedStruct = true
	}

	schema := r.Reflect(v)
	schema.Version = ""
	marshalJSON, err := schema.MarshalJSON()
	if err != nil {
		return ""
	}

	return string(marshalJSON)
}
