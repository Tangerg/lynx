package tool

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"

	reflectionjsonschema "github.com/invopop/jsonschema"
	validationjsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaDefinitionPrefix = "#/$defs/"
	schemaResourceURL      = "urn:lynx:core:tool:input-schema"
)

// schemaContract couples the model-visible schema with the compiled validator
// used at the invocation boundary. Keeping both representations together makes
// it impossible for advertised and enforced contracts to drift.
type schemaContract struct {
	raw      json.RawMessage
	compiled *validationjsonschema.Schema
}

// Schema derives a strict JSON Schema Draft 2020-12 contract for T. It follows
// encoding/json field names and options and the upstream invopop/jsonschema tag
// dialect. Structs reject additional properties by default.
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

	definition, err := reflectSchema(typeOf)
	if err != nil {
		return schemaContract{}, fmt.Errorf("tool: derive schema for %v: %w", typeOf, err)
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		return schemaContract{}, fmt.Errorf("tool: marshal schema for %v: %w", typeOf, err)
	}
	compiled, err := compileSchema(raw)
	if err != nil {
		return schemaContract{}, fmt.Errorf("tool: compile schema for %v: %w", typeOf, err)
	}
	return schemaContract{raw: raw, compiled: compiled}, nil
}

// reflectSchema translates the upstream reflector's panic-based unsupported
// type signal into the error contract expected by constructors in this package.
// References remain enabled so recursive Go types are represented safely.
func reflectSchema(typeOf reflect.Type) (schema *reflectionjsonschema.Schema, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("unsupported schema type: %v", recovered)
		}
	}()
	reflector := reflectionjsonschema.Reflector{
		Anonymous: true,
	}
	definition := reflector.ReflectFromType(typeOf)
	name, referenced := strings.CutPrefix(definition.Ref, schemaDefinitionPrefix)
	if !referenced {
		return definition, nil
	}
	root, exists := definition.Definitions[name]
	if !exists {
		return nil, fmt.Errorf("root definition %q is missing", name)
	}
	expanded := *root
	expanded.Version = definition.Version
	expanded.Definitions = definition.Definitions
	return &expanded, nil
}

func compileSchema(raw json.RawMessage) (*validationjsonschema.Schema, error) {
	document, err := validationjsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode generated schema: %w", err)
	}
	compiler := validationjsonschema.NewCompiler()
	compiler.DefaultDraft(validationjsonschema.Draft2020)
	compiler.AssertFormat()
	if addResourceErr := compiler.AddResource(schemaResourceURL, document); addResourceErr != nil {
		return nil, fmt.Errorf("add generated schema: %w", addResourceErr)
	}
	compiled, err := compiler.Compile(schemaResourceURL)
	if err != nil {
		return nil, err
	}
	return compiled, nil
}

func (s schemaContract) JSON() json.RawMessage {
	return bytes.Clone(s.raw)
}

func (s schemaContract) decode[T any](arguments string) (T, error) {
	var input T
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	value, err := validationjsonschema.UnmarshalJSON(strings.NewReader(arguments))
	if err != nil {
		return input, err
	}
	if s.compiled == nil {
		return input, errors.New("tool: input schema is not compiled")
	}
	if err := s.compiled.Validate(value); err != nil {
		return input, fmt.Errorf("arguments violate input schema: %w", err)
	}
	if err := jsonv2.Unmarshal([]byte(arguments), &input, jsonv2.RejectUnknownMembers(true)); err != nil {
		return input, err
	}
	return input, nil
}
