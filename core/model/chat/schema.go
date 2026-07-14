package chat

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// stringSchemaOf is the frozen legacy Chat schema generator. New tool schema
// ownership lives in the tools module; this helper disappears with the legacy
// core/model/chat package in P6.
func stringSchemaOf(value any) (string, error) {
	if value == nil {
		return "", errors.New("schema value must not be nil")
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
	schema := reflector.Reflect(value)
	if schema == nil {
		return "", fmt.Errorf("reflect schema for %T", value)
	}
	schema.Version = ""
	raw, err := schema.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshal schema: %w", err)
	}
	return string(raw), nil
}
