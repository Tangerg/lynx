package runs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// InterruptKind discriminates the application-owned durable interrupt
// envelope. Executor adapters must persist and restore this exact union; they
// may not infer a kind by inspecting arbitrary prompt fields.
type InterruptKind string

const (
	ApprovalInterruptKind InterruptKind = "approval"
	QuestionInterruptKind InterruptKind = "question"
)

// ApprovalPrompt is the complete durable plan for one gated tool call.
// Arguments are the effective arguments after PreToolUse rewriting, so a
// continuation (including one restored after restart) can resume without
// running the hook or policy decision a second time.
type ApprovalPrompt struct {
	CallID      string           `json:"callId,omitempty"`
	ToolName    string           `json:"toolName"`
	Arguments   string           `json:"arguments"`
	SafetyClass tool.SafetyClass `json:"safetyClass"`
	Risk        tool.RiskLevel   `json:"risk,omitempty"`
	Reason      string           `json:"reason,omitempty"`
}

// QuestionPrompt is the complete durable plan for a question-producing tool
// call. ToolName and Arguments preserve the logical call that owns the
// question; Questions are the client-facing answer schema.
type QuestionPrompt struct {
	ToolName  string         `json:"toolName"`
	Arguments string         `json:"arguments"`
	Questions []QuestionSpec `json:"questions"`
}

// QuestionSpec is one required answer field. An empty Options slice means
// free-text; otherwise 2-4 unique options are accepted.
type QuestionSpec struct {
	Question    string               `json:"question"`
	Header      string               `json:"header,omitempty"`
	Options     []QuestionOptionSpec `json:"options,omitempty"`
	MultiSelect bool                 `json:"multiSelect,omitempty"`
}

type QuestionOptionSpec struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Interrupt is the only JSON payload allowed in a runtime Suspension prompt.
// Exactly one payload must be present and must match Kind.
type Interrupt struct {
	Kind     InterruptKind   `json:"kind"`
	Approval *ApprovalPrompt `json:"approval,omitempty"`
	Question *QuestionPrompt `json:"question,omitempty"`
}

// Tool returns the logical tool call that owns this interrupt.
func (i Interrupt) Tool() (name, arguments string) {
	switch i.Kind {
	case ApprovalInterruptKind:
		if i.Approval != nil {
			return i.Approval.ToolName, i.Approval.Arguments
		}
	case QuestionInterruptKind:
		if i.Question != nil {
			return i.Question.ToolName, i.Question.Arguments
		}
	}
	return "", ""
}

// Validate rejects malformed or ambiguous envelopes before they become
// durable process state or application events.
func (i Interrupt) Validate() error {
	switch i.Kind {
	case ApprovalInterruptKind:
		if i.Approval == nil || i.Question != nil {
			return errors.New("runs: malformed approval interrupt")
		}
		return i.Approval.validate()
	case QuestionInterruptKind:
		if i.Question == nil || i.Approval != nil {
			return errors.New("runs: malformed question interrupt")
		}
		return i.Question.validate()
	default:
		return fmt.Errorf("runs: unknown interrupt kind %q", i.Kind)
	}
}

func (p ApprovalPrompt) validate() error {
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("runs: approval tool name is required")
	}
	if err := validateArguments(p.Arguments); err != nil {
		return fmt.Errorf("runs: approval arguments: %w", err)
	}
	if !p.SafetyClass.Valid() {
		return fmt.Errorf("runs: unknown approval safety class %q", p.SafetyClass)
	}
	if p.Risk != "" && !p.Risk.Valid() {
		return fmt.Errorf("runs: unknown approval risk %q", p.Risk)
	}
	return nil
}

func (p QuestionPrompt) validate() error {
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("runs: question tool name is required")
	}
	if err := validateArguments(p.Arguments); err != nil {
		return fmt.Errorf("runs: question arguments: %w", err)
	}
	if len(p.Questions) < 1 || len(p.Questions) > 4 {
		return fmt.Errorf("runs: question count must be between 1 and 4, got %d", len(p.Questions))
	}
	for index, question := range p.Questions {
		if err := question.validate(); err != nil {
			return fmt.Errorf("runs: question %d: %w", index, err)
		}
	}
	return nil
}

func (q QuestionSpec) validate() error {
	if strings.TrimSpace(q.Question) == "" {
		return errors.New("text is required")
	}
	if utf8.RuneCountInString(q.Header) > 12 {
		return errors.New("header must be at most 12 characters")
	}
	if len(q.Options) == 0 {
		if q.MultiSelect {
			return errors.New("multiSelect requires options")
		}
		return nil
	}
	if len(q.Options) < 2 || len(q.Options) > 4 {
		return fmt.Errorf("option count must be between 2 and 4, got %d", len(q.Options))
	}
	seen := make(map[string]struct{}, len(q.Options))
	for _, option := range q.Options {
		label := strings.TrimSpace(option.Label)
		if label == "" {
			return errors.New("option label is required")
		}
		if label != option.Label {
			return fmt.Errorf("option label %q has surrounding whitespace", option.Label)
		}
		if _, ok := seen[label]; ok {
			return fmt.Errorf("duplicate option label %q", label)
		}
		seen[label] = struct{}{}
	}
	return nil
}

func validateArguments(arguments string) error {
	if strings.TrimSpace(arguments) == "" {
		return errors.New("JSON value is required")
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("JSON object is required")
	}
	return nil
}

// DecodeInterrupt performs the adapter-boundary decode exactly once. It
// rejects unknown fields, trailing JSON and every malformed union shape.
func DecodeInterrupt(raw []byte) (Interrupt, error) {
	var interrupt Interrupt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&interrupt); err != nil {
		return Interrupt{}, fmt.Errorf("runs: decode interrupt: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Interrupt{}, errors.New("runs: decode interrupt: trailing JSON")
		}
		return Interrupt{}, fmt.Errorf("runs: decode interrupt: %w", err)
	}
	if err := interrupt.Validate(); err != nil {
		return Interrupt{}, err
	}
	return interrupt, nil
}
