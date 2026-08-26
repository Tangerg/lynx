package interactioninput

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// EncodePrompt converts one validated product interrupt to its strict executor
// boundary representation.
func EncodePrompt(prompt runs.Interrupt) (json.RawMessage, error) {
	if err := prompt.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(promptWireFrom(prompt))
	if err != nil {
		return nil, fmt.Errorf("agentexec interaction input codec: encode prompt: %w", err)
	}
	return encoded, nil
}

// DecodePrompt restores an application interrupt from persisted executor input
// JSON. Field names must match exactly; duplicate, unknown, and trailing values
// are rejected.
func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	var wire interruptWire
	if err := decode(raw, &wire); err != nil {
		return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: decode interrupt: %w", err)
	}
	interrupt, err := wire.interrupt()
	if err != nil {
		return runs.Interrupt{}, err
	}
	if err := interrupt.Validate(); err != nil {
		return runs.Interrupt{}, err
	}
	return interrupt, nil
}

// DecodeResolution restores a typed user decision from persisted agent-process
// response JSON. It applies the same exact-field and single-value contract as
// [DecodePrompt].
func DecodeResolution(raw []byte) (interrupt.Resolution, error) {
	var wire ResolutionPayload
	if err := decode(raw, &wire); err != nil {
		return interrupt.Resolution{}, fmt.Errorf("agentexec interaction input codec: decode resolution: %w", err)
	}
	return wire.Resolution()
}

// EncodeResolution converts a typed human decision to the JSON the executor
// validates against its pending-input response schema before continuing.
func EncodeResolution(resolution interrupt.Resolution) (json.RawMessage, error) {
	if resolution.RememberScope != "" && !resolution.RememberScope.Valid() {
		return nil, fmt.Errorf("agentexec interaction input codec: unknown remember scope %q", resolution.RememberScope)
	}
	encoded, err := json.Marshal(ResolutionPayload{
		Approved: resolution.Approved, Arguments: resolution.Arguments, Answers: resolution.Answers,
		Reason: resolution.Reason, RememberScope: resolution.RememberScope,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec interaction input codec: encode resolution: %w", err)
	}
	return encoded, nil
}

func decode(raw []byte, target any) error {
	if err := validateExactJSONFieldNames(raw, reflect.TypeOf(target)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func validateExactJSONFieldNames(raw []byte, targetType reflect.Type) error {
	tokenDecoder := json.NewDecoder(bytes.NewReader(raw))
	tokenDecoder.UseNumber()
	if err := validateUniqueJSONNames(tokenDecoder, "$"); err != nil {
		return err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateJSONFieldNames(value, targetType, "$")
}

func validateUniqueJSONNames(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, structured := token.(json.Delim)
	if !structured {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return fmt.Errorf("object member at %s has a non-string name", path)
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("duplicate field %q at %s", name, path)
			}
			seen[name] = struct{}{}
			if err := validateUniqueJSONNames(decoder, path+"."+name); err != nil {
				return err
			}
		}
	case '[':
		for index := 0; decoder.More(); index++ {
			if err := validateUniqueJSONNames(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delimiter, path)
	}
	_, err = decoder.Token()
	return err
}

func validateJSONFieldNames(value any, targetType reflect.Type, path string) error {
	targetType = dereferenceJSONType(targetType)
	if value == nil || targetType == reflect.TypeFor[json.RawMessage]() {
		return nil
	}
	switch targetType.Kind() {
	case reflect.Struct:
		return validateJSONObjectFields(value, targetType, path)
	case reflect.Slice, reflect.Array:
		return validateJSONArrayElements(value, targetType.Elem(), path)
	}
	return nil
}

func dereferenceJSONType(targetType reflect.Type) reflect.Type {
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	return targetType
}

func validateJSONObjectFields(value any, targetType reflect.Type, path string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	fields := jsonFieldTypes(targetType)
	for name, child := range object {
		fieldType, found := fields[name]
		if !found {
			return fmt.Errorf("field %q at %s does not match the exact JSON contract", name, path)
		}
		if err := validateJSONFieldNames(child, fieldType, path+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func jsonFieldTypes(structType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, structType.NumField())
	for index := range structType.NumField() {
		field := structType.Field(index)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func validateJSONArrayElements(value any, elementType reflect.Type, path string) error {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	for index, child := range values {
		if err := validateJSONFieldNames(child, elementType, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

type interruptWire struct {
	Kind     interrupt.Kind      `json:"kind"`
	Approval *approvalPromptWire `json:"approval,omitempty"`
	Question *questionPromptWire `json:"question,omitempty"`
}

type approvalPromptWire struct {
	CallID       string           `json:"callId,omitempty"`
	ToolName     string           `json:"toolName"`
	Arguments    string           `json:"arguments"`
	SafetyClass  tool.SafetyClass `json:"safetyClass"`
	Risk         tool.RiskLevel   `json:"risk,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Rememberable bool             `json:"rememberable,omitempty"`
}

type questionPromptWire struct {
	CallID    string                  `json:"callId,omitempty"`
	ToolName  string                  `json:"toolName"`
	Arguments string                  `json:"arguments"`
	Fields    []questionFieldSpecWire `json:"fields"`
}

type questionFieldSpecWire struct {
	Prompt      string               `json:"prompt"`
	Header      string               `json:"header,omitempty"`
	Options     []questionOptionWire `json:"options,omitempty"`
	Multiple    bool                 `json:"multiple,omitempty"`
	AllowCustom bool                 `json:"allowCustom,omitempty"`
}

type questionOptionWire struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func promptWireFrom(interrupt runs.Interrupt) interruptWire {
	result := interruptWire{Kind: interrupt.Kind}
	if prompt := interrupt.Approval; prompt != nil {
		result.Approval = &approvalPromptWire{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := interrupt.Question; prompt != nil {
		result.Question = &questionPromptWire{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Fields: questionFieldWiresFrom(prompt.Fields),
		}
	}
	return result
}

func (i interruptWire) interrupt() (runs.Interrupt, error) {
	if !i.Kind.Valid() {
		return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown interrupt kind %q", i.Kind)
	}
	result := runs.Interrupt{Kind: i.Kind}
	if prompt := i.Approval; prompt != nil {
		if !prompt.SafetyClass.Valid() {
			return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown safety class %q", prompt.SafetyClass)
		}
		if prompt.Risk != "" && !prompt.Risk.Valid() {
			return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown risk level %q", prompt.Risk)
		}
		result.Approval = &runs.ApprovalPrompt{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := i.Question; prompt != nil {
		result.Question = &runs.QuestionPrompt{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Fields: questionFieldSpecsFrom(i.Question.Fields),
		}
	}
	return result, nil
}

func questionFieldWiresFrom(specs []runs.QuestionFieldSpec) []questionFieldSpecWire {
	if specs == nil {
		return nil
	}
	result := make([]questionFieldSpecWire, len(specs))
	for index, spec := range specs {
		result[index] = questionFieldSpecWire{
			Prompt: spec.Prompt, Header: spec.Header, Multiple: spec.Multiple,
			AllowCustom: spec.AllowCustom,
			Options:     questionOptionWiresFrom(spec.Options),
		}
	}
	return result
}

func questionOptionWiresFrom(options []runs.QuestionOptionSpec) []questionOptionWire {
	if options == nil {
		return nil
	}
	result := make([]questionOptionWire, len(options))
	for index, option := range options {
		result[index] = questionOptionWire{Label: option.Label, Description: option.Description}
	}
	return result
}

func questionFieldSpecsFrom(specs []questionFieldSpecWire) []runs.QuestionFieldSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make([]runs.QuestionFieldSpec, len(specs))
	for index, spec := range specs {
		result[index] = runs.QuestionFieldSpec{
			Prompt: spec.Prompt, Header: spec.Header, Multiple: spec.Multiple,
			AllowCustom: spec.AllowCustom,
			Options:     questionOptionsFrom(spec.Options),
		}
	}
	return result
}

func questionOptionsFrom(options []questionOptionWire) []runs.QuestionOptionSpec {
	if len(options) == 0 {
		return nil
	}
	result := make([]runs.QuestionOptionSpec, len(options))
	for index, option := range options {
		result[index] = runs.QuestionOptionSpec{Label: option.Label, Description: option.Description}
	}
	return result
}

// ResolutionPayload is the exact technical response shape used by the executor
// continuation codec. Callers should use [EncodeResolution] and
// [DecodeResolution].
type ResolutionPayload struct {
	Approved      bool           `json:"approved"`
	Arguments     string         `json:"arguments,omitempty"`
	Answers       [][]string     `json:"answers,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	RememberScope approval.Scope `json:"remember_scope,omitempty"`
}

// Resolution converts the validated technical payload to its Domain value.
func (r ResolutionPayload) Resolution() (interrupt.Resolution, error) {
	if r.RememberScope != "" && !r.RememberScope.Valid() {
		return interrupt.Resolution{}, fmt.Errorf("agentexec interaction input codec: unknown remember scope %q", r.RememberScope)
	}
	answers := r.Answers
	if len(answers) == 0 {
		answers = nil
	}
	return interrupt.Resolution{
		Approved: r.Approved, Arguments: r.Arguments, Answers: answers,
		Reason: r.Reason, RememberScope: r.RememberScope,
	}, nil
}
