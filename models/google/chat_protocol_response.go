package google

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

const (
	protocolNativeFinishReasonKey = "google/native_finish_reason"
	protocolSafetyRatingsKey      = "google/safety_ratings"
	protocolFinishMessageKey      = "google/finish_message"
	protocolModelVersionKey       = "google/model_version"
	protocolToolUseTokensKey      = "google/tool_use_prompt_tokens"
	protocolUsageKey              = "google/usage"
	protocolPromptFeedbackKey     = "google/prompt_feedback"
)

type protocolResponseMapper struct {
	partOffsets map[int]int
}

func newProtocolResponseMapper() *protocolResponseMapper {
	return &protocolResponseMapper{partOffsets: make(map[int]int)}
}

func (m *protocolResponseMapper) mapResponse(requestModel string, response *genai.GenerateContentResponse) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("google: nil response")
	}
	modelName := response.ModelVersion
	if modelName == "" {
		modelName = requestModel
	}
	mapped := &corechat.Response{
		ID:      response.ResponseID,
		Model:   modelName,
		Choices: make([]corechat.Choice, 0, len(response.Candidates)),
	}
	indices := make(map[int]struct{}, len(response.Candidates))
	for position, candidate := range response.Candidates {
		if candidate == nil {
			return nil, fmt.Errorf("google: candidates[%d]: nil candidate", position)
		}
		index := int(candidate.Index)
		if _, duplicate := indices[index]; duplicate {
			return nil, fmt.Errorf("google: candidates[%d]: duplicate index %d", position, index)
		}
		indices[index] = struct{}{}
		choice, err := m.mapCandidate(index, candidate)
		if err != nil {
			return nil, fmt.Errorf("google: candidates[%d]: %w", position, err)
		}
		mapped.Choices = append(mapped.Choices, choice)
	}
	if response.UsageMetadata != nil {
		mapped.Usage = mapProtocolUsage(response.UsageMetadata)
		if err := mapped.SetExtension(protocolUsageKey, protocolUsageExtensionFrom(response.UsageMetadata)); err != nil {
			return nil, err
		}
		if response.UsageMetadata.ToolUsePromptTokenCount != 0 {
			if err := mapped.SetExtension(protocolToolUseTokensKey, int64(response.UsageMetadata.ToolUsePromptTokenCount)); err != nil {
				return nil, err
			}
		}
	}
	if response.ModelVersion != "" {
		if err := mapped.SetExtension(protocolModelVersionKey, response.ModelVersion); err != nil {
			return nil, err
		}
	}
	if response.PromptFeedback != nil {
		if err := mapped.SetExtension(protocolPromptFeedbackKey, response.PromptFeedback); err != nil {
			return nil, err
		}
	}
	if err := mapped.Validate(); err != nil {
		return nil, fmt.Errorf("google: mapped response: %w", err)
	}
	return mapped, nil
}

func (m *protocolResponseMapper) mapCandidate(index int, candidate *genai.Candidate) (corechat.Choice, error) {
	choice := corechat.Choice{Index: index, FinishReason: normalizeProtocolFinishReason(candidate.FinishReason)}
	if candidate.FinishReason != "" {
		if err := choice.SetExtension(protocolNativeFinishReasonKey, candidate.FinishReason); err != nil {
			return corechat.Choice{}, err
		}
	}
	if len(candidate.SafetyRatings) > 0 {
		if err := choice.SetExtension(protocolSafetyRatingsKey, candidate.SafetyRatings); err != nil {
			return corechat.Choice{}, err
		}
	}
	if candidate.FinishMessage != "" {
		if err := choice.SetExtension(protocolFinishMessageKey, candidate.FinishMessage); err != nil {
			return corechat.Choice{}, err
		}
	}
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return choice, nil
	}

	offset := m.partOffsets[index]
	parts := make([]corechat.Part, 0, len(candidate.Content.Parts))
	for partIndex, part := range candidate.Content.Parts {
		if part == nil {
			return corechat.Choice{}, fmt.Errorf("content.parts[%d]: nil part", partIndex)
		}
		mapped, err := mapProtocolCandidatePart(index, offset+partIndex, part)
		if err != nil {
			return corechat.Choice{}, fmt.Errorf("content.parts[%d]: %w", partIndex, err)
		}
		parts = append(parts, mapped)
	}
	m.partOffsets[index] = offset + len(candidate.Content.Parts)
	choice.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	return choice, nil
}

func mapProtocolCandidatePart(choiceIndex, partIndex int, part *genai.Part) (corechat.Part, error) {
	switch {
	case part.FunctionCall != nil:
		if part.FunctionCall.Name == "" {
			return corechat.Part{}, errors.New("function call has no name")
		}
		arguments, err := protocolJSON(part.FunctionCall.Args)
		if err != nil {
			return corechat.Part{}, fmt.Errorf("function call arguments: %w", err)
		}
		id := part.FunctionCall.ID
		if id == "" {
			id = fmt.Sprintf("google/%d/%d", choiceIndex, partIndex)
		}
		return corechat.NewToolCallPart(corechat.ToolCall{ID: id, Name: part.FunctionCall.Name, Arguments: arguments}), nil
	case part.Thought || len(part.ThoughtSignature) > 0:
		return corechat.NewReasoningPart(part.Text, part.ThoughtSignature), nil
	case part.Text != "":
		return corechat.NewTextPart(part.Text), nil
	case part.InlineData != nil:
		value, err := media.NewBytes(part.InlineData.MIMEType, part.InlineData.Data)
		if err != nil {
			return corechat.Part{}, err
		}
		value.Name = part.InlineData.DisplayName
		return corechat.NewMediaPart(value), nil
	case part.FileData != nil:
		value, err := media.NewURI(part.FileData.MIMEType, part.FileData.FileURI)
		if err != nil {
			return corechat.Part{}, err
		}
		value.Name = part.FileData.DisplayName
		return corechat.NewMediaPart(value), nil
	default:
		return corechat.Part{}, errors.New("unsupported or empty response part")
	}
}

func protocolJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func normalizeProtocolFinishReason(reason genai.FinishReason) corechat.FinishReason {
	switch reason {
	case "", genai.FinishReasonUnspecified:
		return ""
	case genai.FinishReasonStop:
		return corechat.FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return corechat.FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent, genai.FinishReasonSPII,
		genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent:
		return corechat.FinishReasonContentFilter
	case genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall:
		return corechat.FinishReasonToolCalls
	default:
		return corechat.FinishReasonOther
	}
}

func mapProtocolUsage(usage *genai.GenerateContentResponseUsageMetadata) corechat.Usage {
	// Gemini reports tool-result prompt tokens and thought tokens outside the
	// similarly named prompt/candidate counters. Core totals include both, while
	// cache and reasoning remain optional breakdowns.
	mapped := corechat.Usage{
		InputTokens:  int64(usage.PromptTokenCount) + int64(usage.ToolUsePromptTokenCount),
		OutputTokens: int64(usage.CandidatesTokenCount) + int64(usage.ThoughtsTokenCount),
	}
	if usage.ThoughtsTokenCount != 0 {
		value := int64(usage.ThoughtsTokenCount)
		mapped.ReasoningTokens = &value
	}
	if usage.CachedContentTokenCount != 0 {
		value := int64(usage.CachedContentTokenCount)
		mapped.CacheReadInputTokens = &value
	}
	return mapped
}

type protocolUsageExtension struct {
	PromptTokenCount        int32 `json:"prompt_token_count,omitempty"`
	CandidatesTokenCount    int32 `json:"candidates_token_count,omitempty"`
	ThoughtsTokenCount      int32 `json:"thoughts_token_count,omitempty"`
	ToolUsePromptTokenCount int32 `json:"tool_use_prompt_token_count,omitempty"`
	CachedContentTokenCount int32 `json:"cached_content_token_count,omitempty"`
	TotalTokenCount         int32 `json:"total_token_count,omitempty"`
}

func protocolUsageExtensionFrom(usage *genai.GenerateContentResponseUsageMetadata) protocolUsageExtension {
	return protocolUsageExtension{
		PromptTokenCount:        usage.PromptTokenCount,
		CandidatesTokenCount:    usage.CandidatesTokenCount,
		ThoughtsTokenCount:      usage.ThoughtsTokenCount,
		ToolUsePromptTokenCount: usage.ToolUsePromptTokenCount,
		CachedContentTokenCount: usage.CachedContentTokenCount,
		TotalTokenCount:         usage.TotalTokenCount,
	}
}
