package deepseek

import (
	"errors"
	"fmt"
	"regexp"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

const RequestExtensionKey = "deepseek/request"

var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ThinkingMode selects whether DeepSeek emits reasoning_content before its
// final answer. The API defaults to enabled when the field is omitted.
type ThinkingMode string

const (
	ThinkingEnabled  ThinkingMode = "enabled"
	ThinkingDisabled ThinkingMode = "disabled"
)

// ThinkingConfig controls DeepSeek's hybrid thinking mode.
type ThinkingConfig struct {
	Type ThinkingMode `json:"type"`
}

func (t ThinkingConfig) Validate() error {
	switch t.Type {
	case ThinkingEnabled, ThinkingDisabled:
		return nil
	default:
		return fmt.Errorf("thinking.type has unsupported value %q", t.Type)
	}
}

// ReasoningEffort controls the amount of reasoning performed in thinking
// mode. DeepSeek V4 Flash supports all three levels. V4 Pro currently accepts
// low but maps it to high.
type ReasoningEffort string

const (
	ReasoningEffortLow  ReasoningEffort = "low"
	ReasoningEffortHigh ReasoningEffort = "high"
	ReasoningEffortMax  ReasoningEffort = "max"
)

// ToolChoiceMode controls whether DeepSeek may select a tool. To force one
// function, set [ToolChoice.FunctionName] instead of Mode.
type ToolChoiceMode string

const (
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceRequired ToolChoiceMode = "required"
)

// ToolChoice represents DeepSeek's string-or-named-function tool_choice
// union without exposing the OpenAI SDK type through this provider package.
type ToolChoice struct {
	Mode         ToolChoiceMode `json:"mode,omitempty"`
	FunctionName string         `json:"function_name,omitempty"`
}

// RequestOptions contains documented DeepSeek Chat Completions fields that
// have no provider-neutral Core equivalent. Store it in
// [chat.Options.Extensions] under [RequestExtensionKey].
type RequestOptions struct {
	Thinking        *ThinkingConfig `json:"thinking,omitempty"`
	ReasoningEffort ReasoningEffort `json:"reasoning_effort,omitempty"`
	ToolChoice      *ToolChoice     `json:"tool_choice,omitempty"`
	LogProbs        *bool           `json:"logprobs,omitempty"`
	TopLogProbs     *int64          `json:"top_logprobs,omitempty"`
	IncludeUsage    *bool           `json:"include_usage,omitempty"`
	UserID          string          `json:"user_id,omitempty"`
}

type requestDialect struct {
	defaults corechat.Options
}

func (r requestDialect) prepareRequest(request *corechat.Request, target *openai.CompatibleRequest) error {
	fields, _, err := request.Options.Extensions.Decode[map[string]any](RequestExtensionKey)
	if err != nil {
		return fmt.Errorf("extension %q: %w", RequestExtensionKey, err)
	}
	if _, exists := fields["response_format"]; exists {
		return fmt.Errorf("extension %q field %q is owned by options.output_format", RequestExtensionKey, "response_format")
	}
	options, _, err := request.Options.Extensions.Decode[RequestOptions](RequestExtensionKey)
	if err != nil {
		return fmt.Errorf("extension %q: %w", RequestExtensionKey, err)
	}
	effective, err := r.defaults.Resolve(request.Options)
	if err != nil {
		return fmt.Errorf("options: %w", err)
	}
	if err := options.ValidateFor(effective, request.Tools, target.Stream()); err != nil {
		return err
	}

	if options.Thinking != nil {
		if err := target.SetExtraField("thinking", options.Thinking); err != nil {
			return err
		}
	}
	if options.ReasoningEffort != "" {
		if err := target.SetExtraField("reasoning_effort", options.ReasoningEffort); err != nil {
			return err
		}
	}
	if options.ToolChoice != nil {
		var value any = options.ToolChoice.Mode
		if options.ToolChoice.FunctionName != "" {
			value = map[string]any{
				"type": "function",
				"function": map[string]string{
					"name": options.ToolChoice.FunctionName,
				},
			}
		}
		if err := target.SetExtraField("tool_choice", value); err != nil {
			return err
		}
	}
	if options.LogProbs != nil {
		if err := target.SetExtraField("logprobs", *options.LogProbs); err != nil {
			return err
		}
	}
	if options.TopLogProbs != nil {
		if err := target.SetExtraField("top_logprobs", *options.TopLogProbs); err != nil {
			return err
		}
	}
	if options.IncludeUsage != nil {
		if err := target.SetExtraField("stream_options", map[string]bool{
			"include_usage": *options.IncludeUsage,
		}); err != nil {
			return err
		}
	}
	if options.UserID != "" {
		if err := target.SetExtraField("user_id", options.UserID); err != nil {
			return err
		}
	}
	return nil
}

func (r RequestOptions) ValidateFor(generation corechat.Options, tools []corechat.ToolDefinition, stream bool) error {
	thinkingEnabled := r.Thinking == nil || r.Thinking.Type != ThinkingDisabled
	if r.Thinking != nil {
		if err := r.Thinking.Validate(); err != nil {
			return err
		}
	}
	switch r.ReasoningEffort {
	case "", ReasoningEffortLow, ReasoningEffortHigh, ReasoningEffortMax:
	default:
		return fmt.Errorf("reasoning_effort has unsupported value %q", r.ReasoningEffort)
	}
	if !thinkingEnabled && r.ReasoningEffort != "" {
		return errors.New("reasoning_effort requires thinking.type=enabled")
	}
	if generation.FrequencyPenalty != nil {
		return errors.New("options.frequency_penalty is deprecated and unsupported by DeepSeek")
	}
	if generation.PresencePenalty != nil {
		return errors.New("options.presence_penalty is deprecated and unsupported by DeepSeek")
	}
	if thinkingEnabled && generation.Temperature != nil {
		return errors.New("options.temperature has no effect while DeepSeek thinking is enabled")
	}
	if thinkingEnabled && generation.TopP != nil {
		return errors.New("options.top_p has no effect while DeepSeek thinking is enabled")
	}
	if len(generation.Stop) > 16 {
		return errors.New("options.stop must contain at most 16 sequences for DeepSeek")
	}
	if len(tools) > 128 {
		return errors.New("tools must contain at most 128 functions for DeepSeek")
	}

	if err := r.ToolChoice.ValidateFor(tools); err != nil {
		return fmt.Errorf("tool_choice: %w", err)
	}
	if r.TopLogProbs != nil {
		if *r.TopLogProbs < 0 || *r.TopLogProbs > 20 {
			return errors.New("top_logprobs must be between 0 and 20")
		}
		if r.LogProbs == nil || !*r.LogProbs {
			return errors.New("top_logprobs requires logprobs=true")
		}
	}
	if r.IncludeUsage != nil && !stream {
		return errors.New("include_usage is valid only for streaming requests")
	}
	if r.UserID != "" {
		if len(r.UserID) > 512 {
			return errors.New("user_id must contain at most 512 characters")
		}
		if !userIDPattern.MatchString(r.UserID) {
			return errors.New("user_id may contain only ASCII letters, digits, hyphens, and underscores")
		}
	}
	return nil
}

func (t *ToolChoice) ValidateFor(tools []corechat.ToolDefinition) error {
	if t == nil {
		return nil
	}
	if t.Mode != "" && t.FunctionName != "" {
		return errors.New("mode and function_name are mutually exclusive")
	}
	if t.Mode == "" && t.FunctionName == "" {
		return errors.New("mode or function_name is required")
	}
	if t.Mode != "" {
		switch t.Mode {
		case ToolChoiceNone:
			return nil
		case ToolChoiceAuto, ToolChoiceRequired:
			if len(tools) == 0 {
				return fmt.Errorf("mode %q requires at least one tool", t.Mode)
			}
			return nil
		default:
			return fmt.Errorf("mode has unsupported value %q", t.Mode)
		}
	}
	for index := range tools {
		if tools[index].Name == t.FunctionName {
			return nil
		}
	}
	return fmt.Errorf("function_name %q does not match a declared tool", t.FunctionName)
}
