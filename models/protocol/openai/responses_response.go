package openai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	corechat "github.com/Tangerg/scope/core/chat"
)

func mapResponsesResponse(response *responses.Response) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("openai responses: nil response")
	}
	parts, hasToolCalls, err := responsesOutputParts(response.Output)
	if err != nil {
		return nil, err
	}
	output := &corechat.Output{FinishReason: responsesFinishReason(response, hasToolCalls)}
	if len(parts) != 0 {
		message := corechat.NewAssistantMessage(parts...)
		output.Message = &message
	}
	mapped := &corechat.Response{
		Output: output,
		Metadata: &corechat.ResponseMetadata{
			ID: response.ID, Model: string(response.Model), Usage: responsesUsage(response.Usage),
		},
	}
	if err := mapped.Metadata.Set(ResponsesResponseExtensionKey, response); err != nil {
		return nil, fmt.Errorf("openai responses: preserve native response: %w", err)
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai responses: response: %w", err)
	}
	return mapped, nil
}

func responsesOutputParts(output []responses.ResponseOutputItemUnion) ([]corechat.Part, bool, error) {
	parts := make([]corechat.Part, 0, len(output))
	hasToolCall := false
	for index := range output {
		item := output[index]
		switch item.Type {
		case "message":
			message := item.AsMessage()
			for _, content := range message.Content {
				if content.Type == "output_text" && content.Text != "" {
					parts = append(parts, corechat.NewTextPart(content.Text))
				}
			}
		case "reasoning":
			reasoning := item.AsReasoning()
			text := joinResponsesReasoning(reasoning)
			signature, encodeErr := encodeResponsesReasoningFrame(reasoning.ToParam())
			if encodeErr != nil {
				return nil, false, fmt.Errorf("openai responses: output[%d] reasoning: %w", index, encodeErr)
			}
			parts = append(parts, corechat.NewReasoningPart(text, signature))
		case "function_call":
			call := item.AsFunctionCall()
			id := call.CallID
			if id == "" {
				id = call.ID
			}
			if id == "" || call.Name == "" {
				return nil, false, fmt.Errorf("openai responses: output[%d] function call lacks ID or name", index)
			}
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{ID: id, Name: call.Name, Arguments: call.Arguments}))
			hasToolCall = true
		}
	}
	return parts, hasToolCall, nil
}

func joinResponsesReasoning(reasoning responses.ResponseReasoningItem) string {
	var text strings.Builder
	if len(reasoning.Content) != 0 {
		for _, content := range reasoning.Content {
			text.WriteString(content.Text)
		}
	} else {
		for _, summary := range reasoning.Summary {
			text.WriteString(summary.Text)
		}
	}
	return text.String()
}

func responsesFinishReason(response *responses.Response, hasToolCall bool) corechat.FinishReason {
	if hasToolCall {
		return corechat.FinishReasonToolCalls
	}
	if response.Status == "incomplete" {
		switch response.IncompleteDetails.Reason {
		case "max_output_tokens":
			return corechat.FinishReasonLength
		case "content_filter":
			return corechat.FinishReasonContentFilter
		default:
			return corechat.FinishReasonOther
		}
	}
	if response.Status != "completed" {
		return corechat.FinishReasonOther
	}
	return corechat.FinishReasonStop
}

func responsesUsage(usage responses.ResponseUsage) corechat.Usage {
	result := corechat.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}
	if usage.OutputTokensDetails.ReasoningTokens > 0 {
		value := usage.OutputTokensDetails.ReasoningTokens
		result.ReasoningTokens = &value
	}
	if usage.InputTokensDetails.CachedTokens > 0 {
		value := usage.InputTokensDetails.CachedTokens
		result.CacheReadInputTokens = &value
	}
	return result
}
