package tool

import (
	"bytes"
	"encoding"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
	timeType          = reflect.TypeFor[time.Time]()
)

type schemaNode struct {
	Type                 string                `json:"type,omitzero"`
	Description          string                `json:"description,omitzero"`
	Format               string                `json:"format,omitzero"`
	Properties           map[string]schemaNode `json:"properties,omitzero"`
	Required             []string              `json:"required,omitzero"`
	Items                *schemaNode           `json:"items,omitzero"`
	Enum                 []string              `json:"enum,omitzero"`
	MinLength            *int                  `json:"minLength,omitzero"`
	MaxLength            *int                  `json:"maxLength,omitzero"`
	Pattern              string                `json:"pattern,omitzero"`
	Minimum              *float64              `json:"minimum,omitzero"`
	Maximum              *float64              `json:"maximum,omitzero"`
	MinItems             *int                  `json:"minItems,omitzero"`
	MaxItems             *int                  `json:"maxItems,omitzero"`
	AdditionalProperties any                   `json:"additionalProperties,omitzero"`
	pattern              *regexp.Regexp
}

type schemaContract struct {
	root schemaNode
	raw  json.RawMessage
}

type schemaBuilder struct {
	visiting map[reflect.Type]bool
}

type jsonField struct {
	name     string
	explicit bool
	skipped  bool
	optional bool
}

// Schema returns a strict, fully inlined JSON Schema for T. Struct fields use
// their encoding/json names. Fields without omitempty or omitzero are required.
// JSON field options outside that pair are rejected because this schema subset
// cannot advertise their decoding semantics exactly. The jsonschema tag
// supports required, enum, minLength, maxLength, pattern, minimum, maximum,
// minItems, and maxItems; jsonschema_description supplies field descriptions.
func Schema[T any]() (json.RawMessage, error) {
	contract, err := newSchemaContract(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	return contract.JSON(), nil
}

func newSchemaContract(typeOf reflect.Type) (schemaContract, error) {
	if typeOf == nil {
		return schemaContract{}, errors.New("tool: nil type has no JSON schema")
	}
	builder := schemaBuilder{visiting: make(map[reflect.Type]bool)}
	node, err := builder.build(typeOf)
	if err != nil {
		return schemaContract{}, fmt.Errorf("tool: derive schema for %v: %w", typeOf, err)
	}
	encoded, err := json.Marshal(node)
	if err != nil {
		return schemaContract{}, fmt.Errorf("tool: marshal schema for %s: %w", typeOf, err)
	}
	return schemaContract{root: node, raw: json.RawMessage(encoded)}, nil
}

func (s schemaContract) JSON() json.RawMessage {
	return bytes.Clone(s.raw)
}

func (s schemaContract) decode[T any](arguments string) (T, error) {
	var input T
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	raw := []byte(arguments)
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return input, err
	}
	if err := s.consumeEOF(decoder); err != nil {
		return input, err
	}
	if err := s.root.validate(value, "arguments"); err != nil {
		return input, fmt.Errorf("arguments violate input schema: %w", err)
	}
	if err := jsonv2.Unmarshal(raw, &input, jsonv2.RejectUnknownMembers(true)); err != nil {
		return input, err
	}
	return input, nil
}

func (schemaContract) consumeEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func (b schemaBuilder) build(typeOf reflect.Type) (schemaNode, error) {
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
		if b.visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		b.visiting[typeOf] = true
		items, err := b.build(typeOf.Elem())
		delete(b.visiting, typeOf)
		if err != nil {
			return schemaNode{}, err
		}
		node := schemaNode{Type: "array", Items: new(items)}
		if typeOf.Kind() == reflect.Array {
			node.MinItems = new(typeOf.Len())
			node.MaxItems = new(typeOf.Len())
		}
		return node, nil
	case reflect.Map:
		if typeOf.Key().Kind() != reflect.String && !typeOf.Key().Implements(textMarshalerType) {
			return schemaNode{}, fmt.Errorf("map key %s is not JSON object-compatible", typeOf.Key())
		}
		if b.visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		b.visiting[typeOf] = true
		values, err := b.build(typeOf.Elem())
		delete(b.visiting, typeOf)
		if err != nil {
			return schemaNode{}, err
		}
		return schemaNode{Type: "object", AdditionalProperties: values}, nil
	case reflect.Struct:
		return b.buildStruct(typeOf)
	default:
		return schemaNode{}, fmt.Errorf("unsupported JSON type %s", typeOf)
	}
}

func (b schemaBuilder) buildStruct(typeOf reflect.Type) (schemaNode, error) {
	if b.visiting[typeOf] {
		return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
	}
	b.visiting[typeOf] = true
	defer delete(b.visiting, typeOf)

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

		jsonField, err := parseJSONField(field)
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if jsonField.skipped {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && !jsonField.explicit && fieldType.Kind() == reflect.Struct {
			embedded, err := b.buildStruct(fieldType)
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

		property, err := b.build(field.Type)
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		property.Description = field.Tag.Get("jsonschema_description")
		explicitlyRequired, err := property.applyTag(field.Tag.Get("jsonschema"))
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if _, duplicate := node.Properties[jsonField.name]; duplicate {
			return schemaNode{}, fmt.Errorf("duplicate JSON property %q", jsonField.name)
		}
		node.Properties[jsonField.name] = property
		if !jsonField.optional || explicitlyRequired {
			node.Required = append(node.Required, jsonField.name)
		}
	}
	return node, nil
}

func parseJSONField(field reflect.StructField) (jsonField, error) {
	tag := field.Tag.Get("json")
	name, options, _ := strings.Cut(tag, ",")
	if name == "-" {
		return jsonField{skipped: true}, nil
	}
	result := jsonField{name: field.Name}
	if name != "" {
		result.name = name
		result.explicit = true
	}
	for option := range strings.SplitSeq(options, ",") {
		switch option {
		case "":
		case "omitempty", "omitzero":
			result.optional = true
		default:
			return jsonField{}, fmt.Errorf("unsupported json option %q", option)
		}
	}
	return result, nil
}

func (node *schemaNode) applyTag(tag string) (bool, error) {
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
			if node.Type != "string" {
				return false, errors.New("enum requires a string field")
			}
			node.Enum = append(node.Enum, value)
		case "minLength":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MinLength = new(parsed)
		case "maxLength":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MaxLength = new(parsed)
		case "pattern":
			if !hasValue || value == "" {
				return false, errors.New("pattern requires a value")
			}
			compiled, err := regexp.Compile(value)
			if err != nil {
				return false, fmt.Errorf("pattern must be a valid regular expression: %w", err)
			}
			node.Pattern = value
			node.pattern = compiled
		case "minimum":
			parsed, err := schemaTagFloat(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.Minimum = new(parsed)
		case "maximum":
			parsed, err := schemaTagFloat(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.Maximum = new(parsed)
		case "minItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MinItems = new(parsed)
		case "maxItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			node.MaxItems = new(parsed)
		default:
			return false, fmt.Errorf("unsupported jsonschema option %q", key)
		}
	}
	if (node.MinLength != nil || node.MaxLength != nil || node.Pattern != "") && node.Type != "string" {
		return false, errors.New("minLength, maxLength, and pattern require a string field")
	}
	if node.MinLength != nil && node.MaxLength != nil && *node.MinLength > *node.MaxLength {
		return false, errors.New("minLength exceeds maxLength")
	}
	if (node.Minimum != nil || node.Maximum != nil) && node.Type != "integer" && node.Type != "number" {
		return false, errors.New("minimum and maximum require a numeric field")
	}
	if node.Minimum != nil && node.Maximum != nil && *node.Minimum > *node.Maximum {
		return false, errors.New("minimum exceeds maximum")
	}
	if (node.MinItems != nil || node.MaxItems != nil) && node.Type != "array" {
		return false, errors.New("minItems and maxItems require an array field")
	}
	if node.MinItems != nil && node.MaxItems != nil && *node.MinItems > *node.MaxItems {
		return false, errors.New("minItems exceeds maxItems")
	}
	return required, nil
}

func (node schemaNode) validate(value any, path string) error {
	switch node.Type {
	case "":
		return nil
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, name := range node.Required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(object)) {
			property, known := node.Properties[name]
			if known {
				if err := property.validate(object[name], path+"."+name); err != nil {
					return err
				}
				continue
			}
			switch additional := node.AdditionalProperties.(type) {
			case nil:
				continue
			case bool:
				if !additional {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			case schemaNode:
				if err := additional.validate(object[name], path+"."+name); err != nil {
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
		if node.MinItems != nil && len(array) < *node.MinItems {
			return fmt.Errorf("%s must contain at least %d items", path, *node.MinItems)
		}
		if node.MaxItems != nil && len(array) > *node.MaxItems {
			return fmt.Errorf("%s must contain at most %d items", path, *node.MaxItems)
		}
		if node.Items != nil {
			for index, item := range array {
				if err := node.Items.validate(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
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
		if node.MinLength != nil && length < *node.MinLength {
			return fmt.Errorf("%s must contain at least %d characters", path, *node.MinLength)
		}
		if node.MaxLength != nil && length > *node.MaxLength {
			return fmt.Errorf("%s must contain at most %d characters", path, *node.MaxLength)
		}
		if node.pattern != nil && !node.pattern.MatchString(text) {
			return fmt.Errorf("%s must match %q", path, node.Pattern)
		}
		if len(node.Enum) > 0 && !slices.Contains(node.Enum, text) {
			return fmt.Errorf("%s must be one of %v", path, node.Enum)
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
			return fmt.Errorf("%s must be a %s", path, node.Type)
		}
		parsed, err := number.Float64()
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return fmt.Errorf("%s must be a finite %s", path, node.Type)
		}
		if node.Type == "integer" && math.Trunc(parsed) != parsed {
			return fmt.Errorf("%s must be an integer", path)
		}
		if node.Minimum != nil && parsed < *node.Minimum {
			return fmt.Errorf("%s must be at least %v", path, *node.Minimum)
		}
		if node.Maximum != nil && parsed > *node.Maximum {
			return fmt.Errorf("%s must be at most %v", path, *node.Maximum)
		}
		return nil
	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, node.Type)
	}
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

func schemaTagFloat(key, value string, hasValue bool) (float64, error) {
	if !hasValue {
		return 0, fmt.Errorf("%s requires a value", key)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return parsed, nil
}
