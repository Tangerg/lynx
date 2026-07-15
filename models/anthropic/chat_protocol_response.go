package anthropic

import (
	"errors"
	"fmt"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const protocolCitationDeltaKey = "anthropic/citation_delta"

func mapProtocolMessage(message *anthropicsdk.Message) (*corechat.Response, error) {
	if message == nil {
		return nil, errors.New("anthropic: nil response")
	}
	parts, redacted, err := mapProtocolContent(message.Content)
	if err != nil {
		return nil, err
	}
	choice := corechat.Choice{
		Index:        0,
		FinishReason: normalizeProtocolStopReason(message.StopReason),
	}
	if err := choice.SetExtension(protocolNativeStopReasonKey, message.StopReason); err != nil {
		return nil, err
	}
	if len(parts) > 0 {
		choice.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
		if err := setProtocolRedacted(&choice.Message.Metadata, redacted); err != nil {
			return nil, err
		}
	} else if len(redacted) > 0 {
		if err := choice.SetExtension(protocolRedactedReasoningDelta, protocolRedactedValue(redacted)); err != nil {
			return nil, err
		}
	}
	response := &corechat.Response{
		ID:      message.ID,
		Model:   string(message.Model),
		Choices: []corechat.Choice{choice},
		Usage:   mapProtocolUsage(message.Usage),
	}
	if message.StopSequence != "" {
		if err := response.SetExtension(protocolStopSequenceKey, message.StopSequence); err != nil {
			return nil, err
		}
	}
	if err := response.SetExtension(protocolUsageKey, message.Usage); err != nil {
		return nil, err
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("anthropic: mapped response: %w", err)
	}
	return response, nil
}

func mapProtocolContent(blocks []anthropicsdk.ContentBlockUnion) ([]corechat.Part, []string, error) {
	parts := make([]corechat.Part, 0, len(blocks))
	redacted := make([]string, 0)
	for i := range blocks {
		block := blocks[i]
		switch block.Type {
		case "text":
			if block.Text != "" {
				parts = append(parts, corechat.NewTextPart(block.Text))
			}
		case "thinking":
			if block.Thinking == "" && block.Signature == "" {
				return nil, nil, fmt.Errorf("anthropic: content[%d]: empty thinking block", i)
			}
			parts = append(parts, corechat.NewReasoningPart(block.Thinking, []byte(block.Signature)))
		case "redacted_thinking":
			if block.Data == "" {
				return nil, nil, fmt.Errorf("anthropic: content[%d]: empty redacted thinking block", i)
			}
			redacted = append(redacted, block.Data)
		case "tool_use":
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			}))
		default:
			return nil, nil, fmt.Errorf("anthropic: content[%d]: unsupported block type %q", i, block.Type)
		}
	}
	return parts, redacted, nil
}

func mapProtocolUsage(usage anthropicsdk.Usage) corechat.Usage {
	mapped := corechat.Usage{
		InputTokens:  protocolTotalInputTokens(usage.InputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens),
		OutputTokens: usage.OutputTokens,
	}
	if usage.OutputTokensDetails.JSON.ThinkingTokens.Valid() || usage.OutputTokensDetails.ThinkingTokens != 0 {
		value := usage.OutputTokensDetails.ThinkingTokens
		mapped.ReasoningTokens = &value
	}
	if usage.CacheReadInputTokens != 0 || usage.JSON.CacheReadInputTokens.Valid() {
		value := usage.CacheReadInputTokens
		mapped.CacheReadInputTokens = &value
	}
	if usage.CacheCreationInputTokens != 0 || usage.JSON.CacheCreationInputTokens.Valid() {
		value := usage.CacheCreationInputTokens
		mapped.CacheWriteInputTokens = &value
	}
	return mapped
}

func protocolTotalInputTokens(uncached, cacheRead, cacheWrite int64) int64 {
	// Anthropic reports fresh, cache-read, and cache-write input as disjoint
	// counters. Core InputTokens is the total whose optional cache fields are
	// breakdowns, so normalize instead of copying the similarly named field.
	return uncached + cacheRead + cacheWrite
}

func normalizeProtocolStopReason(reason anthropicsdk.StopReason) corechat.FinishReason {
	switch reason {
	case "":
		return ""
	case anthropicsdk.StopReasonEndTurn, anthropicsdk.StopReasonStopSequence:
		return corechat.FinishReasonStop
	case anthropicsdk.StopReasonMaxTokens:
		return corechat.FinishReasonLength
	case anthropicsdk.StopReasonToolUse:
		return corechat.FinishReasonToolCalls
	case anthropicsdk.StopReasonRefusal:
		return corechat.FinishReasonContentFilter
	default:
		return corechat.FinishReasonOther
	}
}

func setProtocolRedacted(target *metadata.Map, values []string) error {
	if len(values) == 0 {
		return nil
	}
	return target.Set(protocolRedactedReasoningKey, protocolRedactedValue(values))
}

func protocolRedactedValue(values []string) any {
	if len(values) == 1 {
		return values[0]
	}
	return values
}

type protocolStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type protocolStreamState struct {
	id              string
	model           string
	tools           map[int64]protocolStreamTool
	pendingRedacted []string
	usage           corechat.Usage
}

func newProtocolStreamState() *protocolStreamState {
	return &protocolStreamState{tools: make(map[int64]protocolStreamTool)}
}

func (s *protocolStreamState) mapEvent(event anthropicsdk.MessageStreamEventUnion) (*corechat.Response, bool, error) {
	response := &corechat.Response{ID: s.id, Model: s.model}
	var choice *corechat.Choice
	include := false

	switch value := event.AsAny().(type) {
	case anthropicsdk.MessageStartEvent:
		s.id = value.Message.ID
		s.model = string(value.Message.Model)
		response.ID = s.id
		response.Model = s.model
		s.usage = mapProtocolUsage(value.Message.Usage)
		response.Usage = s.usage
		if err := response.SetExtension(protocolUsageKey, value.Message.Usage); err != nil {
			return nil, false, err
		}
		if len(value.Message.Content) > 0 {
			parts, redacted, err := mapProtocolContent(value.Message.Content)
			if err != nil {
				return nil, false, err
			}
			s.pendingRedacted = append(s.pendingRedacted, redacted...)
			if len(parts) > 0 {
				message, err := s.protocolMessage(parts)
				if err != nil {
					return nil, false, err
				}
				choice = &corechat.Choice{Index: 0, Message: message}
			}
		}
		include = true

	case anthropicsdk.ContentBlockStartEvent:
		part, hasPart, err := s.mapBlockStart(value)
		if err != nil {
			return nil, false, err
		}
		if hasPart {
			message, err := s.protocolMessage([]corechat.Part{part})
			if err != nil {
				return nil, false, err
			}
			choice = &corechat.Choice{Index: 0, Message: message}
			include = true
		}

	case anthropicsdk.ContentBlockDeltaEvent:
		mapped, hasPart, extension, err := s.mapBlockDelta(value)
		if err != nil {
			return nil, false, err
		}
		if hasPart {
			message, err := s.protocolMessage([]corechat.Part{mapped})
			if err != nil {
				return nil, false, err
			}
			choice = &corechat.Choice{Index: 0, Message: message}
			include = true
		} else if extension != nil {
			choice = &corechat.Choice{Index: 0}
			if err := choice.SetExtension(protocolCitationDeltaKey, extension); err != nil {
				return nil, false, err
			}
			include = true
		}

	case anthropicsdk.MessageDeltaEvent:
		choice = &corechat.Choice{Index: 0, FinishReason: normalizeProtocolStopReason(value.Delta.StopReason)}
		if value.Delta.StopReason != "" {
			if err := choice.SetExtension(protocolNativeStopReasonKey, value.Delta.StopReason); err != nil {
				return nil, false, err
			}
		}
		if len(s.pendingRedacted) > 0 {
			if err := choice.SetExtension(protocolRedactedReasoningDelta, protocolRedactedValue(s.pendingRedacted)); err != nil {
				return nil, false, err
			}
			s.pendingRedacted = nil
		}
		s.mergeDeltaUsage(value.Usage)
		response.Usage = s.usage
		if err := response.SetExtension(protocolUsageKey, value.Usage); err != nil {
			return nil, false, err
		}
		if value.Delta.StopSequence != "" {
			if err := response.SetExtension(protocolStopSequenceKey, value.Delta.StopSequence); err != nil {
				return nil, false, err
			}
		}
		if choice.FinishReason == "" && len(choice.Extensions) == 0 {
			choice = nil
		}
		include = choice != nil || len(response.Extensions) > 0

	case anthropicsdk.ContentBlockStopEvent, anthropicsdk.MessageStopEvent:
		return nil, false, nil

	default:
		return nil, false, fmt.Errorf("anthropic: unsupported stream event %q", event.Type)
	}

	if choice != nil {
		response.Choices = []corechat.Choice{*choice}
	}
	if !include {
		return nil, false, nil
	}
	if err := response.Validate(); err != nil {
		return nil, false, fmt.Errorf("anthropic: mapped stream response: %w", err)
	}
	return response, true, nil
}

func (s *protocolStreamState) mapBlockStart(event anthropicsdk.ContentBlockStartEvent) (corechat.Part, bool, error) {
	block := event.ContentBlock
	switch block.Type {
	case "text":
		if block.Text == "" {
			return corechat.Part{}, false, nil
		}
		return corechat.NewTextPart(block.Text), true, nil
	case "thinking":
		if block.Thinking == "" && block.Signature == "" {
			return corechat.Part{}, false, nil
		}
		return corechat.NewReasoningPart(block.Thinking, []byte(block.Signature)), true, nil
	case "redacted_thinking":
		if block.Data == "" {
			return corechat.Part{}, false, errors.New("anthropic: empty redacted thinking block")
		}
		s.pendingRedacted = append(s.pendingRedacted, block.Data)
		return corechat.Part{}, false, nil
	case "tool_use":
		tool := s.tools[event.Index]
		tool.id = block.ID
		tool.name = block.Name
		s.tools[event.Index] = tool
		if tool.id == "" || tool.name == "" {
			return corechat.Part{}, false, errors.New("anthropic: tool_use start requires ID and name")
		}
		arguments := tool.pendingArguments
		tool.pendingArguments = ""
		s.tools[event.Index] = tool
		return corechat.NewToolCallPart(corechat.ToolCall{ID: tool.id, Name: tool.name, Arguments: arguments}), true, nil
	default:
		return corechat.Part{}, false, fmt.Errorf("anthropic: unsupported stream block type %q", block.Type)
	}
}

func (s *protocolStreamState) mapBlockDelta(event anthropicsdk.ContentBlockDeltaEvent) (corechat.Part, bool, any, error) {
	switch delta := event.Delta.AsAny().(type) {
	case anthropicsdk.TextDelta:
		if delta.Text == "" {
			return corechat.Part{}, false, nil, nil
		}
		return corechat.NewTextPart(delta.Text), true, nil, nil
	case anthropicsdk.ThinkingDelta:
		if delta.Thinking == "" {
			return corechat.Part{}, false, nil, nil
		}
		return corechat.NewReasoningPart(delta.Thinking, nil), true, nil, nil
	case anthropicsdk.SignatureDelta:
		if delta.Signature == "" {
			return corechat.Part{}, false, nil, nil
		}
		return corechat.NewReasoningPart("", []byte(delta.Signature)), true, nil, nil
	case anthropicsdk.InputJSONDelta:
		tool := s.tools[event.Index]
		tool.pendingArguments += delta.PartialJSON
		s.tools[event.Index] = tool
		if tool.id == "" || tool.name == "" {
			return corechat.Part{}, false, nil, nil
		}
		arguments := tool.pendingArguments
		tool.pendingArguments = ""
		s.tools[event.Index] = tool
		return corechat.NewToolCallPart(corechat.ToolCall{ID: tool.id, Name: tool.name, Arguments: arguments}), true, nil, nil
	case anthropicsdk.CitationsDelta:
		return corechat.Part{}, false, delta, nil
	default:
		return corechat.Part{}, false, nil, fmt.Errorf("anthropic: unsupported content delta type %q", event.Delta.Type)
	}
}

func (s *protocolStreamState) protocolMessage(parts []corechat.Part) (*corechat.Message, error) {
	message := &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	if err := setProtocolRedacted(&message.Metadata, s.pendingRedacted); err != nil {
		return nil, err
	}
	s.pendingRedacted = nil
	return message, nil
}

func (s *protocolStreamState) mergeDeltaUsage(usage anthropicsdk.MessageDeltaUsage) {
	if usage.InputTokens > 0 || usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 {
		s.usage.InputTokens = protocolTotalInputTokens(usage.InputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
	}
	s.usage.OutputTokens = usage.OutputTokens
	if usage.OutputTokensDetails.ThinkingTokens != 0 || usage.OutputTokensDetails.JSON.ThinkingTokens.Valid() {
		value := usage.OutputTokensDetails.ThinkingTokens
		s.usage.ReasoningTokens = &value
	}
	if usage.CacheReadInputTokens != 0 {
		value := usage.CacheReadInputTokens
		s.usage.CacheReadInputTokens = &value
	}
	if usage.CacheCreationInputTokens != 0 {
		value := usage.CacheCreationInputTokens
		s.usage.CacheWriteInputTokens = &value
	}
}
