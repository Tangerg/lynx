package event

import "time"

// LLMRequestEvent and LLMResponseEvent are best-effort observability
// events the framework defines but does NOT emit — they're integration
// extension points. Adapters wrapping a chat client publish them via
// [core.ProcessContext.Publish] so listeners can wire telemetry / cost
// accounting (see [core.Process.RecordUsage] for budget plumbing).

// LLMRequestEvent describes an outbound LLM call.
type LLMRequestEvent struct {
	BaseEvent
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

func (LLMRequestEvent) EventName() string { return "llm_request" }

func (e LLMRequestEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"model":    e.Model,
		"provider": e.Provider,
		"prompt":   e.Prompt,
	})
}

// LLMResponseEvent describes the result of an LLM call: token counts,
// duration, and any error.
type LLMResponseEvent struct {
	BaseEvent
	Model        string        `json:"model,omitempty"`
	InputTokens  int           `json:"input_tokens,omitempty"`
	OutputTokens int           `json:"output_tokens,omitempty"`
	Duration     time.Duration `json:"-"`
	Err          error         `json:"-"`
}

func (LLMResponseEvent) EventName() string { return "llm_response" }

func (e LLMResponseEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"model":         e.Model,
		"input_tokens":  e.InputTokens,
		"output_tokens": e.OutputTokens,
		"duration_ns":   e.Duration.Nanoseconds(),
		"error":         errString(e.Err),
	})
}
