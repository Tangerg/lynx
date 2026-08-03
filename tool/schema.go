package tool

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
	timeType          = reflect.TypeFor[time.Time]()
)

type schemaNode struct {
	Type                 string                `json:"type,omitempty"`
	Description          string                `json:"description,omitempty"`
	Format               string                `json:"format,omitempty"`
	Properties           map[string]schemaNode `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Items                *schemaNode           `json:"items,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	MinItems             *int                  `json:"minItems,omitempty"`
	MaxItems             *int                  `json:"maxItems,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
}

// Schema returns a strict, fully inlined JSON Schema for T. Struct fields use
// their encoding/json names. The jsonschema tag supports required, enum,
// minItems, and maxItems; jsonschema_description supplies field descriptions.
func Schema[T any]() (string, error) {
	typeOf := reflect.TypeFor[T]()
	node, err := schemaFor(typeOf, make(map[reflect.Type]bool))
	if err != nil {
		return "", fmt.Errorf("tool: derive schema for %s: %w", typeOf, err)
	}
	encoded, err := json.Marshal(node)
	if err != nil {
		return "", fmt.Errorf("tool: marshal schema for %s: %w", typeOf, err)
	}
	return string(encoded), nil
}

// MustSchema is Schema's panicking variant for immutable package-level tool
// definitions.
func MustSchema[T any]() string {
	schema, err := Schema[T]()
	if err != nil {
		panic(err)
	}
	return schema
}

func schemaFor(typeOf reflect.Type, visiting map[reflect.Type]bool) (schemaNode, error) {
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}

	switch {
	case typeOf == rawMessageType:
		return schemaNode{}, nil
	case typeOf == timeType:
		return schemaNode{Type: "string", Format: "date-time"}, nil
	case typeOf.Implements(textMarshalerType) || reflect.PointerTo(typeOf).Implements(textMarshalerType):
		return schemaNode{Type: "string"}, nil
	}

	switch typeOf.Kind() {
	case reflect.Bool:
		return schemaNode{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return schemaNode{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return schemaNode{Type: "number"}, nil
	case reflect.String:
		return schemaNode{Type: "string"}, nil
	case reflect.Interface:
		return schemaNode{}, nil
	case reflect.Slice, reflect.Array:
		if typeOf == rawMessageType {
			return schemaNode{}, nil
		}
		if visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		visiting[typeOf] = true
		items, err := schemaFor(typeOf.Elem(), visiting)
		delete(visiting, typeOf)
		if err != nil {
			return schemaNode{}, err
		}
		node := schemaNode{Type: "array", Items: &items}
		if typeOf.Kind() == reflect.Array {
			length := typeOf.Len()
			node.MinItems = &length
			node.MaxItems = &length
		}
		return node, nil
	case reflect.Map:
		if typeOf.Key().Kind() != reflect.String && !typeOf.Key().Implements(textMarshalerType) {
			return schemaNode{}, fmt.Errorf("map key %s is not JSON object-compatible", typeOf.Key())
		}
		if visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		visiting[typeOf] = true
		values, err := schemaFor(typeOf.Elem(), visiting)
		delete(visiting, typeOf)
		if err != nil {
			return schemaNode{}, err
		}
		return schemaNode{Type: "object", AdditionalProperties: values}, nil
	case reflect.Struct:
		return schemaForStruct(typeOf, visiting)
	default:
		return schemaNode{}, fmt.Errorf("unsupported JSON type %s", typeOf)
	}
}

func schemaForStruct(typeOf reflect.Type, visiting map[reflect.Type]bool) (schemaNode, error) {
	if visiting[typeOf] {
		return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)

	node := schemaNode{
		Type:                 "object",
		Properties:           make(map[string]schemaNode),
		AdditionalProperties: false,
	}
	for index := range typeOf.NumField() {
		field := typeOf.Field(index)
		if field.PkgPath != "" {
			continue
		}

		name, explicit, skip := jsonFieldName(field)
		if skip {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && !explicit && fieldType.Kind() == reflect.Struct {
			embedded, err := schemaForStruct(fieldType, visiting)
			if err != nil {
				return schemaNode{}, err
			}
			for embeddedName, property := range embedded.Properties {
				if _, duplicate := node.Properties[embeddedName]; duplicate {
					return schemaNode{}, fmt.Errorf("embedded field %s duplicates JSON property %q", field.Name, embeddedName)
				}
				node.Properties[embeddedName] = property
			}
			node.Required = append(node.Required, embedded.Required...)
			continue
		}

		property, err := schemaFor(field.Type, visiting)
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		property.Description = field.Tag.Get("jsonschema_description")
		required, err := applySchemaTag(&property, field.Tag.Get("jsonschema"))
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if _, duplicate := node.Properties[name]; duplicate {
			return schemaNode{}, fmt.Errorf("duplicate JSON property %q", name)
		}
		node.Properties[name] = property
		if required {
			node.Required = append(node.Required, name)
		}
	}
	return node, nil
}

func jsonFieldName(field reflect.StructField) (name string, explicit, skip bool) {
	tag := field.Tag.Get("json")
	name, _, _ = strings.Cut(tag, ",")
	if name == "-" {
		return "", false, true
	}
	if name != "" {
		return name, true, false
	}
	return field.Name, false, false
}

func applySchemaTag(node *schemaNode, tag string) (bool, error) {
	var required bool
	for option := range strings.SplitSeq(tag, ",") {
		if option == "" {
			continue
		}
		key, value, hasValue := strings.Cut(option, "=")
		switch key {
		case "required":
			if hasValue {
				return false, errors.New("required does not accept a value")
			}
			required = true
		case "enum":
			if !hasValue || value == "" {
				return false, errors.New("enum requires a value")
			}
			node.Enum = append(node.Enum, value)
		case "minItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MinItems = &parsed
		case "maxItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MaxItems = &parsed
		default:
			return false, fmt.Errorf("unsupported jsonschema option %q", key)
		}
	}
	if (node.MinItems != nil || node.MaxItems != nil) && node.Type != "array" {
		return false, errors.New("minItems and maxItems require an array field")
	}
	if node.MinItems != nil && node.MaxItems != nil && *node.MinItems > *node.MaxItems {
		return false, errors.New("minItems exceeds maxItems")
	}
	return required, nil
}

func schemaTagInt(key, value string, hasValue bool) (int, error) {
	if !hasValue {
		return 0, fmt.Errorf("%s requires a value", key)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return parsed, nil
}
