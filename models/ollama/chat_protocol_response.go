package ollama

import (
	"encoding/json"
	"fmt"
	"time"

	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	// ResponseExtensionKey preserves the complete official Ollama response (or
	// the current official stream chunk), including log probabilities and image
	// output that Core does not normalize.
	ResponseExtensionKey        = "ollama/response"
	protocolNativeDoneReasonKey = "ollama/native_done_reason"
	protocolCreatedAtKey        = "ollama/created_at"
	protocolDurationsKey        = "ollama/durations_ns"
	protocolMetricsKey          = "ollama/metrics"
)

type protocolResponseMapper struct{}

func newProtocolResponseMapper() *protocolResponseMapper {
	return new(protocolResponseMapper)
}

func (p *protocolResponseMapper) mapResponse(requestModel string, response nativeChatResponse) (*corechat.Response, error) {
	modelName := response.Model
	if modelName == "" {
		modelName = requestModel
	}
	mapped := &corechat.Response{
		Metadata: &corechat.ResponseMetadata{
			Model: modelName,
			Usage: corechat.Usage{
				InputTokens:  int64(response.PromptEvalCount),
				OutputTokens: int64(response.EvalCount),
			},
		},
	}
	if err := mapped.Metadata.Set(ResponseExtensionKey, response.raw); err != nil {
		return nil, fmt.Errorf("ollama: preserve native response: %w", err)
	}

	output, present, err := p.mapOutput(response)
	if err != nil {
		return nil, err
	}
	if present {
		mapped.Output = output
	}
	if !response.CreatedAt.IsZero() {
		if err := mapped.Metadata.Set(protocolCreatedAtKey, response.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return nil, err
		}
	}
	if hasProtocolDurations(response.nativeMetrics) {
		durations := map[string]int64{
			"total":       int64(response.TotalDuration),
			"load":        int64(response.LoadDuration),
			"prompt_eval": int64(response.PromptEvalDuration),
			"eval":        int64(response.EvalDuration),
		}
		if err := mapped.Metadata.Set(protocolDurationsKey, durations); err != nil {
			return nil, err
		}
	}
	if response.PromptEvalCount != 0 || response.EvalCount != 0 {
		metrics := protocolMetrics{
			PromptEvalCount: response.PromptEvalCount,
			EvalCount:       response.EvalCount,
		}
		if err := mapped.Metadata.Set(protocolMetricsKey, metrics); err != nil {
			return nil, err
		}
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("ollama: mapped response: %w", err)
	}
	return mapped, nil
}

func (p *protocolResponseMapper) mapOutput(response nativeChatResponse) (*corechat.Output, bool, error) {
	output := &corechat.Output{FinishReason: normalizeProtocolDoneReason(response.DoneReason)}
	if response.DoneReason != "" {
		output.Metadata = &corechat.OutputMetadata{}
		if err := output.Metadata.Set(protocolNativeDoneReasonKey, response.DoneReason); err != nil {
			return nil, false, err
		}
	}

	var parts []corechat.Part
	if response.Message.Thinking != "" {
		parts = append(parts, corechat.NewReasoningPart(response.Message.Thinking, nil))
	}
	if response.Message.Content != "" {
		parts = append(parts, corechat.NewTextPart(response.Message.Content))
	}
	for i := range response.Message.ToolCalls {
		toolCall := response.Message.ToolCalls[i]
		if toolCall.Function.Name == "" {
			return nil, false, fmt.Errorf("ollama: message.tool_calls[%d]: empty function name", i)
		}
		arguments, err := json.Marshal(toolCall.Function.Arguments)
		if err != nil {
			return nil, false, fmt.Errorf("ollama: message.tool_calls[%d].arguments: %w", i, err)
		}
		id := toolCall.ID
		if id == "" {
			id = fmt.Sprintf("%s%d", protocolGeneratedToolPrefix, toolCall.Function.Index)
		}
		parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
			ID:        id,
			Name:      toolCall.Function.Name,
			Arguments: string(arguments),
		}))
	}
	if len(parts) > 0 {
		output.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	}
	present := output.Message != nil || output.FinishReason != "" || output.Metadata != nil
	return output, present, nil
}

func normalizeProtocolDoneReason(reason string) corechat.FinishReason {
	switch reason {
	case "":
		return ""
	case "stop":
		return corechat.FinishReasonStop
	case "length":
		return corechat.FinishReasonLength
	default:
		return corechat.FinishReasonOther
	}
}

func hasProtocolDurations(metrics nativeMetrics) bool {
	return metrics.TotalDuration != 0 || metrics.LoadDuration != 0 ||
		metrics.PromptEvalDuration != 0 || metrics.EvalDuration != 0
}

type protocolMetrics struct {
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}
