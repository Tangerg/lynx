package openai

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"

	corechat "github.com/Tangerg/scope/core/chat"
)

func mapResponsesResponse(response *responses.Response) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("openai responses: nil response")
	}
	parts, hasToolCalls, hasRefusal, err := responsesOutputParts(response.Output)
	if err != nil {
		return nil, err
	}
	output := &corechat.Output{FinishReason: responsesFinishReason(response, hasToolCalls)}
	if hasRefusal {
		output.FinishReason = corechat.FinishReasonRefusal
	}
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
	if response.CreatedAt > 0 {
		mapped.Metadata.CreatedAt = time.Unix(int64(response.CreatedAt), 0).UTC()
	}
	if err := mapped.Metadata.Extra.Set(ResponsesResponseExtensionKey, response); err != nil {
		return nil, fmt.Errorf("openai responses: preserve native response: %w", err)
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("openai responses: response: %w", err)
	}
	return mapped, nil
}

func responsesOutputParts(output []responses.ResponseOutputItemUnion) ([]corechat.Part, bool, bool, error) {
	parts := make([]corechat.Part, 0, len(output))
	hasToolCall := false
	hasRefusal := false
	for index := range output {
		item := output[index]
		switch item.Type {
		case responsesItemTypeMessage:
			message := item.AsMessage()
			for contentIndex := range message.Content {
				content := message.Content[contentIndex]
				switch content.Type {
				case responsesContentTypeText:
					if content.Text == "" {
						continue
					}
					part := corechat.NewTextPart(content.Text)
					for annotationIndex := range content.Annotations {
						citation, include, mapErr := responsesCitation(content.Annotations[annotationIndex])
						if mapErr != nil {
							return nil, false, false, fmt.Errorf("openai responses: output[%d].content[%d].annotations[%d]: %w", index, contentIndex, annotationIndex, mapErr)
						}
						if include {
							part.Citations = append(part.Citations, citation)
						}
					}
					parts = append(parts, part)
				case responsesContentTypeRefusal:
					if content.Refusal != "" {
						parts = append(parts, corechat.NewRefusalPart(content.Refusal))
						hasRefusal = true
					}
				}
			}
		case responsesItemTypeReasoning:
			reasoning := item.AsReasoning()
			text := joinResponsesReasoning(reasoning)
			signature, encodeErr := encodeResponsesReasoningFrame(reasoning.ToParam())
			if encodeErr != nil {
				return nil, false, false, fmt.Errorf("openai responses: output[%d] reasoning: %w", index, encodeErr)
			}
			parts = append(parts, corechat.NewReasoningPart(text, signature))
		case responsesItemTypeFunctionCall:
			call := item.AsFunctionCall()
			id := call.CallID
			if id == "" {
				id = call.ID
			}
			if id == "" || call.Name == "" {
				return nil, false, false, fmt.Errorf("openai responses: output[%d] function call lacks ID or name", index)
			}
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{ID: id, Name: call.Name, Arguments: call.Arguments}))
			hasToolCall = true
		}
	}
	return parts, hasToolCall, hasRefusal, nil
}

func responsesCitation(annotation responses.ResponseOutputTextAnnotationUnion) (corechat.Citation, bool, error) {
	switch typed := annotation.AsAny().(type) {
	case responses.ResponseOutputTextAnnotationURLCitation:
		return corechat.Citation{
			Source: corechat.CitationSource{Kind: corechat.CitationSourceURI, Value: typed.URL},
			Title:  typed.Title,
		}, true, nil
	case responses.ResponseOutputTextAnnotationFileCitation:
		return corechat.Citation{
			Source: corechat.CitationSource{Kind: corechat.CitationSourceReference, Value: typed.FileID},
			Title:  typed.Filename,
		}, true, nil
	case responses.ResponseOutputTextAnnotationContainerFileCitation:
		return corechat.Citation{
			Source: corechat.CitationSource{Kind: corechat.CitationSourceReference, Value: typed.FileID},
			Title:  typed.Filename,
		}, true, nil
	case responses.ResponseOutputTextAnnotationFilePath:
		return corechat.Citation{
			Source: corechat.CitationSource{Kind: corechat.CitationSourceReference, Value: typed.FileID},
		}, true, nil
	case nil:
		return corechat.Citation{}, false, nil
	default:
		return corechat.Citation{}, false, fmt.Errorf("unsupported annotation %T", typed)
	}
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
	for itemIndex := range response.Output {
		if response.Output[itemIndex].Type != responsesItemTypeMessage {
			continue
		}
		for contentIndex := range response.Output[itemIndex].AsMessage().Content {
			if response.Output[itemIndex].AsMessage().Content[contentIndex].Type == responsesContentTypeRefusal {
				return corechat.FinishReasonRefusal
			}
		}
	}
	if response.Status == responses.ResponseStatusIncomplete {
		switch response.IncompleteDetails.Reason {
		case responsesIncompleteMaxTokens:
			return corechat.FinishReasonLength
		case responsesIncompleteFiltered:
			return corechat.FinishReasonContentFilter
		default:
			return corechat.FinishReasonOther
		}
	}
	if response.Status != responses.ResponseStatusCompleted {
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
