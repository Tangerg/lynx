package anthropic

import (
	"errors"
	"fmt"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/lynx/core/chat"
)

const (
	// ResponseExtensionKey preserves the complete official Anthropic response.
	ResponseExtensionKey = "anthropic/response"
	// StreamEventExtensionKey preserves each official Anthropic stream event.
	StreamEventExtensionKey  = "anthropic/stream_event"
	protocolCitationDeltaKey = "anthropic/citation_delta"
)

func mapProtocolMessage(message *anthropicsdk.Message, provider string) (*corechat.Response, error) {
	if message == nil {
		return nil, errors.New("anthropic: nil response")
	}
	parts, err := mapProtocolContent(message.Content, provider)
	if err != nil {
		return nil, err
	}
	output := &corechat.Output{
		FinishReason: normalizeProtocolStopReason(message.StopReason),
		Metadata:     &corechat.OutputMetadata{},
	}
	if err := output.Metadata.Set(protocolNativeStopReasonKey, message.StopReason); err != nil {
		return nil, err
	}
	if len(parts) > 0 {
		output.Message = &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}
	}
	response := &corechat.Response{
		Output: output,
		Metadata: &corechat.ResponseMetadata{
			ID:    message.ID,
			Model: string(message.Model),
			Usage: mapProtocolUsage(message.Usage),
		},
	}
	if err := response.Metadata.Set(protocolResponseExtensionKey(provider), message); err != nil {
		return nil, err
	}
	if message.StopSequence != "" {
		if err := response.Metadata.Set(protocolStopSequenceKey, message.StopSequence); err != nil {
			return nil, err
		}
	}
	if err := response.Metadata.Set(protocolUsageKey, message.Usage); err != nil {
		return nil, err
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("anthropic: mapped response: %w", err)
	}
	return response, nil
}

func mapProtocolContent(blocks []anthropicsdk.ContentBlockUnion, provider string) ([]corechat.Part, error) {
	parts := make([]corechat.Part, 0, len(blocks))
	for i := range blocks {
		block := blocks[i]
		switch block.Type {
		case "text":
			if block.Text != "" {
				parts = append(parts, corechat.NewTextPart(block.Text))
			}
		case "thinking":
			if block.Thinking == "" && block.Signature == "" {
				return nil, fmt.Errorf("anthropic: content[%d]: empty thinking block", i)
			}
			part := corechat.NewReasoningPart(block.Thinking, []byte(block.Signature))
			if err := setProtocolReasoningState(&part, provider, protocolReasoningThinking); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case "redacted_thinking":
			if block.Data == "" {
				return nil, fmt.Errorf("anthropic: content[%d]: empty redacted thinking block", i)
			}
			part := corechat.NewReasoningPart("", []byte(block.Data))
			if err := setProtocolReasoningState(&part, provider, protocolReasoningRedacted); err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case "tool_use":
			parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			}))
		default:
			// Server-tool and future native blocks have no provider-neutral Core
			// part. The complete message remains available under
			// ResponseExtensionKey, so skipping them here is lossless.
			continue
		}
	}
	return parts, nil
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

func setProtocolReasoningState(part *corechat.Part, provider, kind string) error {
	if err := part.Metadata.Set(protocolReasoningProviderKey, provider); err != nil {
		return fmt.Errorf("anthropic: preserve reasoning provider: %w", err)
	}
	if err := part.Metadata.Set(protocolReasoningKindKey, kind); err != nil {
		return fmt.Errorf("anthropic: preserve reasoning kind: %w", err)
	}
	return nil
}

type protocolStreamTool struct {
	id               string
	name             string
	pendingArguments string
}

type protocolStreamState struct {
	provider       string
	streamEventKey string
	id             string
	model          string
	tools          map[int64]protocolStreamTool
	usage          corechat.Usage
}

func newProtocolStreamState(provider string) *protocolStreamState {
	return &protocolStreamState{
		provider:       provider,
		streamEventKey: protocolStreamEventExtensionKey(provider),
		tools:          make(map[int64]protocolStreamTool),
	}
}

func (p *protocolStreamState) mapEvent(event anthropicsdk.MessageStreamEventUnion) (*corechat.Response, bool, error) {
	response := &corechat.Response{Metadata: &corechat.ResponseMetadata{ID: p.id, Model: p.model}}
	if err := response.Metadata.Set(p.streamEventKey, event); err != nil {
		return nil, false, err
	}
	var output *corechat.Output
	include := true

	switch value := event.AsAny().(type) {
	case anthropicsdk.MessageStartEvent:
		p.id = value.Message.ID
		p.model = string(value.Message.Model)
		response.Metadata.ID = p.id
		response.Metadata.Model = p.model
		p.usage = mapProtocolUsage(value.Message.Usage)
		response.Metadata.Usage = p.usage
		if err := response.Metadata.Set(protocolUsageKey, value.Message.Usage); err != nil {
			return nil, false, err
		}
		if len(value.Message.Content) > 0 {
			parts, err := mapProtocolContent(value.Message.Content, p.provider)
			if err != nil {
				return nil, false, err
			}
			if len(parts) > 0 {
				message, err := p.protocolMessage(parts)
				if err != nil {
					return nil, false, err
				}
				output = &corechat.Output{Message: message}
			}
		}
		include = true

	case anthropicsdk.ContentBlockStartEvent:
		part, hasPart, err := p.mapBlockStart(value)
		if err != nil {
			return nil, false, err
		}
		if hasPart {
			message, err := p.protocolMessage([]corechat.Part{part})
			if err != nil {
				return nil, false, err
			}
			output = &corechat.Output{Message: message}
			include = true
		}

	case anthropicsdk.ContentBlockDeltaEvent:
		mapped, hasPart, extension, err := p.mapBlockDelta(value)
		if err != nil {
			return nil, false, err
		}
		if hasPart {
			message, err := p.protocolMessage([]corechat.Part{mapped})
			if err != nil {
				return nil, false, err
			}
			output = &corechat.Output{Message: message}
			include = true
		} else if extension != nil {
			output = &corechat.Output{Metadata: &corechat.OutputMetadata{}}
			if err := output.Metadata.Set(protocolCitationDeltaKey, extension); err != nil {
				return nil, false, err
			}
			include = true
		}

	case anthropicsdk.MessageDeltaEvent:
		output = &corechat.Output{FinishReason: normalizeProtocolStopReason(value.Delta.StopReason)}
		if value.Delta.StopReason != "" {
			output.Metadata = &corechat.OutputMetadata{}
			if err := output.Metadata.Set(protocolNativeStopReasonKey, value.Delta.StopReason); err != nil {
				return nil, false, err
			}
		}
		p.mergeDeltaUsage(value.Usage)
		response.Metadata.Usage = p.usage
		if err := response.Metadata.Set(protocolUsageKey, value.Usage); err != nil {
			return nil, false, err
		}
		if value.Delta.StopSequence != "" {
			if err := response.Metadata.Set(protocolStopSequenceKey, value.Delta.StopSequence); err != nil {
				return nil, false, err
			}
		}
		if output.FinishReason == "" && output.Metadata == nil {
			output = nil
		}
		include = output != nil || len(response.Metadata.Extra) > 0

	case anthropicsdk.ContentBlockStopEvent, anthropicsdk.MessageStopEvent:

	default:
		// Preserve forward-compatible events in StreamEventExtensionKey.
	}

	if output != nil {
		response.Output = output
	}
	if !include {
		return nil, false, nil
	}
	// Usage is a cumulative stream snapshot. Event-only chunks still carry the
	// latest known value so observing native lifecycle events cannot make a
	// consumer'p view of usage move backwards.
	response.Metadata.Usage = p.usage
	if err := response.Validate(); err != nil {
		return nil, false, fmt.Errorf("anthropic: mapped stream response: %w", err)
	}
	return response, true, nil
}

func (p *protocolStreamState) mapBlockStart(event anthropicsdk.ContentBlockStartEvent) (corechat.Part, bool, error) {
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
		part := corechat.NewReasoningPart(block.Thinking, []byte(block.Signature))
		if err := setProtocolReasoningState(&part, p.provider, protocolReasoningThinking); err != nil {
			return corechat.Part{}, false, err
		}
		return part, true, nil
	case "redacted_thinking":
		if block.Data == "" {
			return corechat.Part{}, false, errors.New("anthropic: empty redacted thinking block")
		}
		part := corechat.NewReasoningPart("", []byte(block.Data))
		if err := setProtocolReasoningState(&part, p.provider, protocolReasoningRedacted); err != nil {
			return corechat.Part{}, false, err
		}
		return part, true, nil
	case "tool_use":
		tool := p.tools[event.Index]
		tool.id = block.ID
		tool.name = block.Name
		p.tools[event.Index] = tool
		if tool.id == "" || tool.name == "" {
			return corechat.Part{}, false, errors.New("anthropic: tool_use start requires ID and name")
		}
		arguments := tool.pendingArguments
		tool.pendingArguments = ""
		p.tools[event.Index] = tool
		return corechat.NewToolCallPart(corechat.ToolCall{ID: tool.id, Name: tool.name, Arguments: arguments}), true, nil
	default:
		return corechat.Part{}, false, nil
	}
}

func (p *protocolStreamState) mapBlockDelta(event anthropicsdk.ContentBlockDeltaEvent) (corechat.Part, bool, any, error) {
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
		part := corechat.NewReasoningPart(delta.Thinking, nil)
		if err := setProtocolReasoningState(&part, p.provider, protocolReasoningThinking); err != nil {
			return corechat.Part{}, false, nil, err
		}
		return part, true, nil, nil
	case anthropicsdk.SignatureDelta:
		if delta.Signature == "" {
			return corechat.Part{}, false, nil, nil
		}
		part := corechat.NewReasoningPart("", []byte(delta.Signature))
		if err := setProtocolReasoningState(&part, p.provider, protocolReasoningThinking); err != nil {
			return corechat.Part{}, false, nil, err
		}
		return part, true, nil, nil
	case anthropicsdk.InputJSONDelta:
		tool := p.tools[event.Index]
		tool.pendingArguments += delta.PartialJSON
		p.tools[event.Index] = tool
		if tool.id == "" || tool.name == "" {
			return corechat.Part{}, false, nil, nil
		}
		arguments := tool.pendingArguments
		tool.pendingArguments = ""
		p.tools[event.Index] = tool
		return corechat.NewToolCallPart(corechat.ToolCall{ID: tool.id, Name: tool.name, Arguments: arguments}), true, nil, nil
	case anthropicsdk.CitationsDelta:
		return corechat.Part{}, false, delta, nil
	default:
		return corechat.Part{}, false, nil, nil
	}
}

func (p *protocolStreamState) protocolMessage(parts []corechat.Part) (*corechat.Message, error) {
	return &corechat.Message{Role: corechat.RoleAssistant, Parts: parts}, nil
}

func (p *protocolStreamState) mergeDeltaUsage(usage anthropicsdk.MessageDeltaUsage) {
	if usage.InputTokens > 0 || usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 {
		p.usage.InputTokens = protocolTotalInputTokens(usage.InputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
	}
	p.usage.OutputTokens = usage.OutputTokens
	if usage.OutputTokensDetails.ThinkingTokens != 0 || usage.OutputTokensDetails.JSON.ThinkingTokens.Valid() {
		value := usage.OutputTokensDetails.ThinkingTokens
		p.usage.ReasoningTokens = &value
	}
	if usage.CacheReadInputTokens != 0 {
		value := usage.CacheReadInputTokens
		p.usage.CacheReadInputTokens = &value
	}
	if usage.CacheCreationInputTokens != 0 {
		value := usage.CacheCreationInputTokens
		p.usage.CacheWriteInputTokens = &value
	}
}
