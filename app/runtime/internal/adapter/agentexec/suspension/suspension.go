// Package suspension translates application HITL values at the agent-process
// persistence boundary. The agent runtime stores JSON, while application and
// domain values remain transport-free.
package suspension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Func suspends the current agent process with one application interrupt and
// resumes it with the user's typed response.
type Func func(ctx context.Context, key string, prompt runs.Interrupt) (interrupts.Resolution, error)

// Unavailable reports that the current tool environment cannot suspend for a
// human response.
func Unavailable(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
	return interrupts.Resolution{}, errors.New("agent suspension is unavailable")
}

// Interrupt serializes an application interrupt only at the agent boundary.
func Interrupt(ctx context.Context, key string, prompt runs.Interrupt) (interrupts.Resolution, error) {
	if err := prompt.Validate(); err != nil {
		return interrupts.Resolution{}, err
	}
	response, err := hitl.Interrupt[resolutionWire](ctx, key, promptWireFrom(prompt))
	if err != nil {
		return interrupts.Resolution{}, err
	}
	return response.resolution()
}

// DecodePrompt restores an application interrupt from persisted agent-process
// suspension JSON. Unknown fields and trailing values are rejected.
func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	var wire interruptWire
	if err := decode(raw, &wire); err != nil {
		return runs.Interrupt{}, fmt.Errorf("agent suspension: decode interrupt: %w", err)
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
// response JSON.
func DecodeResolution(raw []byte) (interrupts.Resolution, error) {
	var wire resolutionWire
	if err := decode(raw, &wire); err != nil {
		return interrupts.Resolution{}, fmt.Errorf("agent suspension: decode resolution: %w", err)
	}
	return wire.resolution()
}

// EncodeResolution converts a typed human decision to the JSON the agent
// process validates against its suspension schema before continuing.
func EncodeResolution(resolution interrupts.Resolution) (json.RawMessage, error) {
	if resolution.RememberScope != "" && !resolution.RememberScope.Valid() {
		return nil, fmt.Errorf("agent suspension: unknown remember scope %q", resolution.RememberScope)
	}
	encoded, err := json.Marshal(resolutionWire{
		Approved: resolution.Approved, Arguments: resolution.Arguments, Answer: resolution.Answer,
		Reason: resolution.Reason, RememberScope: rememberScopeWireFrom(resolution.RememberScope),
	})
	if err != nil {
		return nil, fmt.Errorf("agent suspension: encode resolution: %w", err)
	}
	return encoded, nil
}

func decode(raw []byte, target any) error {
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

type interruptWire struct {
	Kind     interruptKindWire   `json:"kind"`
	Approval *approvalPromptWire `json:"approval,omitempty"`
	Question *questionPromptWire `json:"question,omitempty"`
}

type approvalPromptWire struct {
	CallID       string          `json:"callId,omitempty"`
	ToolName     string          `json:"toolName"`
	Arguments    string          `json:"arguments"`
	SafetyClass  safetyClassWire `json:"safetyClass"`
	Risk         riskLevelWire   `json:"risk,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Rememberable bool            `json:"rememberable,omitempty"`
}

type questionPromptWire struct {
	ToolName  string             `json:"toolName"`
	Arguments string             `json:"arguments"`
	Questions []questionSpecWire `json:"questions"`
}

type questionSpecWire struct {
	Question    string               `json:"question"`
	Header      string               `json:"header,omitempty"`
	Options     []questionOptionWire `json:"options,omitempty"`
	MultiSelect bool                 `json:"multiSelect,omitempty"`
}

type questionOptionWire struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func promptWireFrom(interrupt runs.Interrupt) interruptWire {
	result := interruptWire{Kind: interruptKindWireFrom(interrupt.Kind)}
	if prompt := interrupt.Approval; prompt != nil {
		result.Approval = &approvalPromptWire{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: safetyClassWireFrom(prompt.SafetyClass), Risk: riskLevelWireFrom(prompt.Risk), Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := interrupt.Question; prompt != nil {
		result.Question = &questionPromptWire{
			ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Questions: questionWiresFrom(prompt.Questions),
		}
	}
	return result
}

func (wire interruptWire) interrupt() (runs.Interrupt, error) {
	kind, err := wire.Kind.interruptKind()
	if err != nil {
		return runs.Interrupt{}, err
	}
	result := runs.Interrupt{Kind: kind}
	if prompt := wire.Approval; prompt != nil {
		safety, err := prompt.SafetyClass.safetyClass()
		if err != nil {
			return runs.Interrupt{}, err
		}
		risk, err := prompt.Risk.riskLevel()
		if err != nil {
			return runs.Interrupt{}, err
		}
		result.Approval = &runs.ApprovalPrompt{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: safety, Risk: risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := wire.Question; prompt != nil {
		result.Question = &runs.QuestionPrompt{
			ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Questions: questionSpecsFrom(wire.Question.Questions),
		}
	}
	return result, nil
}

func questionWiresFrom(specs []runs.QuestionSpec) []questionSpecWire {
	if specs == nil {
		return nil
	}
	result := make([]questionSpecWire, len(specs))
	for index, spec := range specs {
		result[index] = questionSpecWire{
			Question: spec.Question, Header: spec.Header, MultiSelect: spec.MultiSelect,
			Options: questionOptionWiresFrom(spec.Options),
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

func questionSpecsFrom(specs []questionSpecWire) []runs.QuestionSpec {
	if specs == nil {
		return nil
	}
	result := make([]runs.QuestionSpec, len(specs))
	for index, spec := range specs {
		result[index] = runs.QuestionSpec{
			Question: spec.Question, Header: spec.Header, MultiSelect: spec.MultiSelect,
			Options: questionOptionsFrom(spec.Options),
		}
	}
	return result
}

func questionOptionsFrom(options []questionOptionWire) []runs.QuestionOptionSpec {
	if options == nil {
		return nil
	}
	result := make([]runs.QuestionOptionSpec, len(options))
	for index, option := range options {
		result[index] = runs.QuestionOptionSpec{Label: option.Label, Description: option.Description}
	}
	return result
}

type resolutionWire struct {
	Approved      bool                `json:"approved"`
	Arguments     string              `json:"arguments,omitempty"`
	Answer        map[string][]string `json:"answer,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	RememberScope rememberScopeWire   `json:"remember_scope,omitempty"`
}

func (wire resolutionWire) resolution() (interrupts.Resolution, error) {
	rememberScope, err := wire.RememberScope.scope()
	if err != nil {
		return interrupts.Resolution{}, err
	}
	return interrupts.Resolution{
		Approved: wire.Approved, Arguments: wire.Arguments, Answer: wire.Answer,
		Reason: wire.Reason, RememberScope: rememberScope,
	}, nil
}

type interruptKindWire string

func interruptKindWireFrom(kind runs.InterruptKind) interruptKindWire {
	switch kind {
	case runs.ApprovalInterruptKind:
		return "approval"
	case runs.QuestionInterruptKind:
		return "question"
	default:
		return interruptKindWire(kind)
	}
}

func (wire interruptKindWire) interruptKind() (runs.InterruptKind, error) {
	switch wire {
	case "approval":
		return runs.ApprovalInterruptKind, nil
	case "question":
		return runs.QuestionInterruptKind, nil
	default:
		return "", fmt.Errorf("agent suspension: unknown interrupt kind %q", wire)
	}
}

type safetyClassWire string

func safetyClassWireFrom(class tool.SafetyClass) safetyClassWire {
	switch class {
	case tool.SafetyClassSafe:
		return "safe"
	case tool.SafetyClassWrite:
		return "write"
	case tool.SafetyClassExec:
		return "exec"
	case tool.SafetyClassNetwork:
		return "network"
	default:
		return safetyClassWire(class)
	}
}

func (wire safetyClassWire) safetyClass() (tool.SafetyClass, error) {
	switch wire {
	case "safe":
		return tool.SafetyClassSafe, nil
	case "write":
		return tool.SafetyClassWrite, nil
	case "exec":
		return tool.SafetyClassExec, nil
	case "network":
		return tool.SafetyClassNetwork, nil
	default:
		return "", fmt.Errorf("agent suspension: unknown safety class %q", wire)
	}
}

type riskLevelWire string

func riskLevelWireFrom(risk tool.RiskLevel) riskLevelWire {
	switch risk {
	case "":
		return ""
	case tool.RiskLow:
		return "low"
	case tool.RiskMedium:
		return "medium"
	case tool.RiskHigh:
		return "high"
	default:
		return riskLevelWire(risk)
	}
}

func (wire riskLevelWire) riskLevel() (tool.RiskLevel, error) {
	switch wire {
	case "":
		return "", nil
	case "low":
		return tool.RiskLow, nil
	case "medium":
		return tool.RiskMedium, nil
	case "high":
		return tool.RiskHigh, nil
	default:
		return "", fmt.Errorf("agent suspension: unknown risk level %q", wire)
	}
}

type rememberScopeWire string

func rememberScopeWireFrom(scope approval.Scope) rememberScopeWire {
	switch scope {
	case "":
		return ""
	case approval.ScopeSession:
		return "session"
	case approval.ScopeProject:
		return "project"
	case approval.ScopeGlobal:
		return "global"
	default:
		return rememberScopeWire(scope)
	}
}

func (wire rememberScopeWire) scope() (approval.Scope, error) {
	switch wire {
	case "":
		return "", nil
	case "session":
		return approval.ScopeSession, nil
	case "project":
		return approval.ScopeProject, nil
	case "global":
		return approval.ScopeGlobal, nil
	default:
		return "", fmt.Errorf("agent suspension: unknown remember scope %q", wire)
	}
}
