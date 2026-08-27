package bedrock

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func mapProtocolConverseResponse(model string, output *bedrockruntime.ConverseOutput) (*corechat.Response, error) {
	if output == nil || output.Output == nil {
		return nil, errors.New("bedrock: response has no output")
	}
	messageOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok || messageOutput == nil {
		return nil, errors.New("bedrock: response has no message output")
	}
	parts, err := mapProtocolResponseBlocks(messageOutput.Value.Content)
	if err != nil {
		return nil, err
	}
	modelOutput := &corechat.Output{FinishReason: mapProtocolStopReason(output.StopReason)}
	if len(parts) != 0 {
		message := corechat.NewAssistantMessage(parts...)
		modelOutput.Message = &message
	}
	if modelOutput.FinishReason == corechat.FinishReasonOther {
		modelOutput.Metadata = &corechat.OutputMetadata{}
		if err := modelOutput.Metadata.Set(chatNativeFinishReasonKey, string(output.StopReason)); err != nil {
			return nil, err
		}
	}
	response := &corechat.Response{
		Output:   modelOutput,
		Metadata: &corechat.ResponseMetadata{Model: model, Usage: mapProtocolUsage(output.Usage)},
	}
	if err := response.Metadata.Set(ChatResponseExtensionKey, output); err != nil {
		return nil, fmt.Errorf("bedrock: preserve native response: %w", err)
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("bedrock: response: %w", err)
	}
	return response, nil
}

func mapProtocolResponseBlocks(blocks []types.ContentBlock) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(blocks))
	for index := range blocks {
		switch block := blocks[index].(type) {
		case *types.ContentBlockMemberText:
			if block.Value != "" {
				parts = append(parts, corechat.NewTextPart(block.Value))
			}
		case *types.ContentBlockMemberImage:
			value, err := bedrockImageToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberAudio:
			value, err := bedrockAudioToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberVideo:
			value, err := bedrockVideoToMedia(block.Value)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
			}
			parts = append(parts, corechat.NewMediaPart(value))
		case *types.ContentBlockMemberReasoningContent:
			switch reasoning := block.Value.(type) {
			case *types.ReasoningContentBlockMemberReasoningText:
				if reasoning.Value.Text == nil || reasoning.Value.Signature == nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: reasoning text lacks text or signature", index)
				}
				part, err := NewReasoningPart(*reasoning.Value.Text, []byte(*reasoning.Value.Signature))
				if err != nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
				}
				parts = append(parts, part)
			case *types.ReasoningContentBlockMemberRedactedContent:
				part, err := NewRedactedReasoningPart(reasoning.Value)
				if err != nil {
					return nil, fmt.Errorf("bedrock: response content[%d]: %w", index, err)
				}
				parts = append(parts, part)
			}
		case *types.ContentBlockMemberToolUse:
			if block.Value.ToolUseId == nil || block.Value.Name == nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: tool use lacks ID or name", index)
			}
			arguments, err := json.Marshal(block.Value.Input)
			if err != nil {
				return nil, fmt.Errorf("bedrock: response content[%d]: tool arguments: %w", index, err)
			}
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
				ID: *block.Value.ToolUseId, Name: *block.Value.Name, Arguments: string(arguments),
			}))
		}
	}
	return parts, nil
}

func mapProtocolStopReason(reason types.StopReason) corechat.FinishReason {
	switch reason {
	case types.StopReasonEndTurn, types.StopReasonStopSequence:
		return corechat.FinishReasonStop
	case types.StopReasonMaxTokens:
		return corechat.FinishReasonLength
	case types.StopReasonToolUse:
		return corechat.FinishReasonToolCalls
	case types.StopReasonContentFiltered, types.StopReasonGuardrailIntervened:
		return corechat.FinishReasonContentFilter
	default:
		return corechat.FinishReasonOther
	}
}

func mapProtocolUsage(usage *types.TokenUsage) corechat.Usage {
	if usage == nil {
		return corechat.Usage{}
	}
	result := corechat.Usage{}
	if usage.InputTokens != nil {
		result.InputTokens = int64(*usage.InputTokens)
	}
	if usage.OutputTokens != nil {
		result.OutputTokens = int64(*usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != nil {
		value := int64(*usage.CacheReadInputTokens)
		result.CacheReadInputTokens = &value
	}
	if usage.CacheWriteInputTokens != nil {
		value := int64(*usage.CacheWriteInputTokens)
		result.CacheWriteInputTokens = &value
	}
	return result
}
