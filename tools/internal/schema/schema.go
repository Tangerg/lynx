package schema

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// String returns the strict, fully inlined JSON Schema for value.
func String(value any) (string, error) {
	if value == nil {
		return "", errors.New("value must not be nil")
	}

	reflector := &jsonschema.Reflector{
		Anonymous:      true,
		DoNotReference: true,
	}
	valueType := reflect.TypeOf(value)
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if valueType.Kind() == reflect.Struct {
		reflector.ExpandedStruct = true
	}

	reflected := reflector.Reflect(value)
	if reflected == nil {
		return "", fmt.Errorf("reflect schema for %T", value)
	}
	reflected.Version = ""

	raw, err := reflected.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshal schema: %w", err)
	}
	return string(raw), nil
}
