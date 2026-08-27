package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
)

// InterruptFunc is the consumer-owned capability a tool uses to park the
// current execution on one application interrupt. Tool packages depend only on
// this contract and never on execution internals.
type InterruptFunc func(context.Context, string, Interrupt) (interrupt.Resolution, error)

// InterruptUnavailable is the fail-closed default for a tool environment that
// has no execution interrupt provider.
func InterruptUnavailable(context.Context, string, Interrupt) (interrupt.Resolution, error) {
	return interrupt.Resolution{}, errors.New("runs: execution interrupts are unavailable")
}

// ApprovalPrompt is the complete durable plan for one gated tool call.
// Arguments are the effective arguments after PreToolUse rewriting, so a
// continuation (including one restored after restart) can resume without
// running the hook or policy decision a second time.
type ApprovalPrompt struct {
	CallID      string
	ToolName    string
	Arguments   string
	SafetyClass tool.SafetyClass
	Risk        tool.RiskLevel
	Reason      string
	// Rememberable distinguishes ordinary policy approvals from one-off
	// confirmations such as the doom-loop brake. It must persist with the
	// prompt so a resumed execution cannot accidentally create a standing rule.
	Rememberable bool
}

// QuestionPrompt is the complete durable plan for a question-producing tool
// call. CallID is filled by the execution ACL when the prompt crosses the Tool
// boundary; ToolName and Arguments preserve the logical call for compatibility
// with older checkpoints and non-execution tests. Fields are the client-facing
// answer schema.
type QuestionPrompt struct {
	CallID    string
	ToolName  string
	Arguments string
	Fields    []QuestionFieldSpec
}

// QuestionFieldSpec is one required answer field. An empty Options slice means
// free-text; otherwise 2-4 unique options are accepted.
type QuestionFieldSpec struct {
	Prompt      string
	Header      string
	Options     []QuestionOptionSpec
	Multiple    bool
	AllowCustom bool
}

type QuestionOptionSpec struct {
	Label       string
	Description string
}

// Interrupt is the durable product request for external input. Exactly
// one payload must be present and must match Kind. Executor continuation data is
// deliberately absent.
type Interrupt struct {
	Kind     interrupt.Kind
	Approval *ApprovalPrompt
	Question *QuestionPrompt
}

// Tool returns the logical tool call that owns this interrupt.
func (i Interrupt) Tool() (name, arguments string) {
	switch i.Kind {
	case interrupt.Approval:
		if i.Approval != nil {
			return i.Approval.ToolName, i.Approval.Arguments
		}
	case interrupt.Question:
		if i.Question != nil {
			return i.Question.ToolName, i.Question.Arguments
		}
	}
	return "", ""
}

// Validate rejects malformed or ambiguous envelopes before they become
// a durable Pending aggregate or application events.
func (i Interrupt) Validate() error {
	switch i.Kind {
	case interrupt.Approval:
		if i.Approval == nil || i.Question != nil {
			return errors.New("runs: malformed approval interrupt")
		}
		return i.Approval.validate()
	case interrupt.Question:
		if i.Question == nil || i.Approval != nil {
			return errors.New("runs: malformed question interrupt")
		}
		return i.Question.validate()
	default:
		return fmt.Errorf("runs: unknown interrupt kind %q", i.Kind)
	}
}

func (a ApprovalPrompt) validate() error {
	if strings.TrimSpace(a.ToolName) == "" {
		return errors.New("runs: approval tool name is required")
	}
	if err := validateArguments(a.Arguments); err != nil {
		return fmt.Errorf("runs: approval arguments: %w", err)
	}
	if !a.SafetyClass.Valid() {
		return fmt.Errorf("runs: unknown approval safety class %q", a.SafetyClass)
	}
	if !a.Risk.Valid() {
		return fmt.Errorf("runs: unknown approval risk %q", a.Risk)
	}
	return nil
}

func (q QuestionPrompt) validate() error {
	if q.CallID != strings.TrimSpace(q.CallID) {
		return errors.New("runs: question call ID has surrounding whitespace")
	}
	if strings.TrimSpace(q.ToolName) == "" {
		return errors.New("runs: question tool name is required")
	}
	if err := validateArguments(q.Arguments); err != nil {
		return fmt.Errorf("runs: question arguments: %w", err)
	}
	if len(q.Fields) < 1 || len(q.Fields) > 4 {
		return fmt.Errorf("runs: question field count must be between 1 and 4, got %d", len(q.Fields))
	}
	for index, field := range q.Fields {
		if err := field.validate(); err != nil {
			return fmt.Errorf("runs: question field %d: %w", index, err)
		}
	}
	return nil
}

func (q QuestionFieldSpec) validate() error {
	if strings.TrimSpace(q.Prompt) == "" {
		return errors.New("prompt is required")
	}
	if utf8.RuneCountInString(q.Header) > 12 {
		return errors.New("header must be at most 12 characters")
	}
	if len(q.Options) == 0 {
		if q.Multiple || q.AllowCustom {
			return errors.New("choice settings require options")
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
		return fmt.Errorf("%w: value is required", tool.ErrInvalidArguments)
	}
	_, err := tool.ParseArguments(arguments)
	return err
}
