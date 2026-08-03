package tool

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"unicode/utf8"
)

// validateSchemaValue enforces the schema subset emitted by Schema. Keeping
// generation and validation in this package makes a typed function's advertised
// input contract its actual call boundary without adding a dependency above
// Core or teaching an Agent runtime about Go argument types.
func validateSchemaValue(schema schemaNode, value any, path string) error {
	switch schema.Type {
	case "":
		return nil
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, name := range schema.Required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(object)) {
			property, known := schema.Properties[name]
			if known {
				if err := validateSchemaValue(property, object[name], path+"."+name); err != nil {
					return err
				}
				continue
			}
			switch additional := schema.AdditionalProperties.(type) {
			case nil:
				continue
			case bool:
				if !additional {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			case schemaNode:
				if err := validateSchemaValue(additional, object[name], path+"."+name); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%s has an invalid additional-properties contract", path)
			}
		}
		return nil
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if schema.MinItems != nil && len(array) < *schema.MinItems {
			return fmt.Errorf("%s must contain at least %d items", path, *schema.MinItems)
		}
		if schema.MaxItems != nil && len(array) > *schema.MaxItems {
			return fmt.Errorf("%s must contain at most %d items", path, *schema.MaxItems)
		}
		if schema.Items != nil {
			for index, item := range array {
				if err := validateSchemaValue(*schema.Items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
		return nil
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		length := utf8.RuneCountInString(text)
		if schema.MinLength != nil && length < *schema.MinLength {
			return fmt.Errorf("%s must contain at least %d characters", path, *schema.MinLength)
		}
		if schema.MaxLength != nil && length > *schema.MaxLength {
			return fmt.Errorf("%s must contain at most %d characters", path, *schema.MaxLength)
		}
		if schema.pattern != nil && !schema.pattern.MatchString(text) {
			return fmt.Errorf("%s must match %q", path, schema.Pattern)
		}
		if len(schema.Enum) > 0 && !slices.Contains(schema.Enum, text) {
			return fmt.Errorf("%s must be one of %v", path, schema.Enum)
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
		return nil
	case "integer", "number":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s must be a %s", path, schema.Type)
		}
		parsed, err := number.Float64()
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return fmt.Errorf("%s must be a finite %s", path, schema.Type)
		}
		if schema.Type == "integer" && math.Trunc(parsed) != parsed {
			return fmt.Errorf("%s must be an integer", path)
		}
		if schema.Minimum != nil && parsed < *schema.Minimum {
			return fmt.Errorf("%s must be at least %v", path, *schema.Minimum)
		}
		if schema.Maximum != nil && parsed > *schema.Maximum {
			return fmt.Errorf("%s must be at most %v", path, *schema.Maximum)
		}
		return nil
	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, schema.Type)
	}
}
