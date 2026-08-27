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

func (s schemaBuilder) build(typeOf reflect.Type) (schemaNode, error) {
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
		if s.visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		s.visiting[typeOf] = true
		items, err := s.build(typeOf.Elem())
		delete(s.visiting, typeOf)
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
		if s.visiting[typeOf] {
			return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
		}
		s.visiting[typeOf] = true
		values, err := s.build(typeOf.Elem())
		delete(s.visiting, typeOf)
		if err != nil {
			return schemaNode{}, err
		}
		return schemaNode{Type: "object", AdditionalProperties: values}, nil
	case reflect.Struct:
		return s.buildStruct(typeOf)
	default:
		return schemaNode{}, fmt.Errorf("unsupported JSON type %s", typeOf)
	}
}

func (s schemaBuilder) buildStruct(typeOf reflect.Type) (schemaNode, error) {
	if s.visiting[typeOf] {
		return schemaNode{}, fmt.Errorf("recursive type %s cannot be fully inlined", typeOf)
	}
	s.visiting[typeOf] = true
	defer delete(s.visiting, typeOf)

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

		fieldJSON, err := parseJSONField(field)
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if fieldJSON.skipped {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && !fieldJSON.explicit && fieldType.Kind() == reflect.Struct {
			embedded, buildErr := s.buildStruct(fieldType)
			if buildErr != nil {
				return schemaNode{}, buildErr
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

		property, err := s.build(field.Type)
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		property.Description = field.Tag.Get("jsonschema_description")
		explicitlyRequired, err := property.applyTag(field.Tag.Get("jsonschema"))
		if err != nil {
			return schemaNode{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		if _, duplicate := node.Properties[fieldJSON.name]; duplicate {
			return schemaNode{}, fmt.Errorf("duplicate JSON property %q", fieldJSON.name)
		}
		node.Properties[fieldJSON.name] = property
		if !fieldJSON.optional || explicitlyRequired {
			node.Required = append(node.Required, fieldJSON.name)
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

func (s *schemaNode) applyTag(tag string) (bool, error) {
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
			if s.Type != "string" {
				return false, errors.New("enum requires a string field")
			}
			s.Enum = append(s.Enum, value)
		case "minLength":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.MinLength = new(parsed)
		case "maxLength":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.MaxLength = new(parsed)
		case "pattern":
			if !hasValue || value == "" {
				return false, errors.New("pattern requires a value")
			}
			compiled, err := regexp.Compile(value)
			if err != nil {
				return false, fmt.Errorf("pattern must be a valid regular expression: %w", err)
			}
			s.Pattern = value
			s.pattern = compiled
		case "minimum":
			parsed, err := schemaTagFloat(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.Minimum = new(parsed)
		case "maximum":
			parsed, err := schemaTagFloat(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.Maximum = new(parsed)
		case "minItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.MinItems = new(parsed)
		case "maxItems":
			parsed, err := schemaTagInt(key, value, hasValue)
			if err != nil {
				return false, err
			}
			s.MaxItems = new(parsed)
		default:
			return false, fmt.Errorf("unsupported jsonschema option %q", key)
		}
	}
	if (s.MinLength != nil || s.MaxLength != nil || s.Pattern != "") && s.Type != "string" {
		return false, errors.New("minLength, maxLength, and pattern require a string field")
	}
	if s.MinLength != nil && s.MaxLength != nil && *s.MinLength > *s.MaxLength {
		return false, errors.New("minLength exceeds maxLength")
	}
	if (s.Minimum != nil || s.Maximum != nil) && s.Type != "integer" && s.Type != "number" {
		return false, errors.New("minimum and maximum require a numeric field")
	}
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		return false, errors.New("minimum exceeds maximum")
	}
	if (s.MinItems != nil || s.MaxItems != nil) && s.Type != "array" {
		return false, errors.New("minItems and maxItems require an array field")
	}
	if s.MinItems != nil && s.MaxItems != nil && *s.MinItems > *s.MaxItems {
		return false, errors.New("minItems exceeds maxItems")
	}
	return required, nil
}

func (s schemaNode) validate(value any, path string) error {
	switch s.Type {
	case "":
		return nil
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, name := range s.Required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(object)) {
			property, known := s.Properties[name]
			if known {
				if err := property.validate(object[name], path+"."+name); err != nil {
					return err
				}
				continue
			}
			switch additional := s.AdditionalProperties.(type) {
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
		if s.MinItems != nil && len(array) < *s.MinItems {
			return fmt.Errorf("%s must contain at least %d items", path, *s.MinItems)
		}
		if s.MaxItems != nil && len(array) > *s.MaxItems {
			return fmt.Errorf("%s must contain at most %d items", path, *s.MaxItems)
		}
		if s.Items != nil {
			for index, item := range array {
				if err := s.Items.validate(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
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
		if s.MinLength != nil && length < *s.MinLength {
			return fmt.Errorf("%s must contain at least %d characters", path, *s.MinLength)
		}
		if s.MaxLength != nil && length > *s.MaxLength {
			return fmt.Errorf("%s must contain at most %d characters", path, *s.MaxLength)
		}
		if s.pattern != nil && !s.pattern.MatchString(text) {
			return fmt.Errorf("%s must match %q", path, s.Pattern)
		}
		if len(s.Enum) > 0 && !slices.Contains(s.Enum, text) {
			return fmt.Errorf("%s must be one of %v", path, s.Enum)
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
			return fmt.Errorf("%s must be a %s", path, s.Type)
		}
		parsed, err := number.Float64()
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return fmt.Errorf("%s must be a finite %s", path, s.Type)
		}
		if s.Type == "integer" && math.Trunc(parsed) != parsed {
			return fmt.Errorf("%s must be an integer", path)
		}
		if s.Minimum != nil && parsed < *s.Minimum {
			return fmt.Errorf("%s must be at least %v", path, *s.Minimum)
		}
		if s.Maximum != nil && parsed > *s.Maximum {
			return fmt.Errorf("%s must be at most %v", path, *s.Maximum)
		}
		return nil
	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, s.Type)
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
