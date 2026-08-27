package bedrock

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	corechat "github.com/Tangerg/scope/core/chat"
)

type protocolToolIdentity struct {
	id   string
	name string
}

type protocolChunkAccumulator struct {
	model string
	tools map[int32]protocolToolIdentity
}

func newProtocolChunkAccumulator(model string) *protocolChunkAccumulator {
	return &protocolChunkAccumulator{model: model, tools: make(map[int32]protocolToolIdentity)}
}

func (p *protocolChunkAccumulator) add(event types.ConverseStreamOutput) (*corechat.Response, bool, error) {
	response := &corechat.Response{Metadata: &corechat.ResponseMetadata{Model: p.model}}
	var output *corechat.Output

	switch typed := event.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		tool, ok := typed.Value.Start.(*types.ContentBlockStartMemberToolUse)
		if !ok || typed.Value.ContentBlockIndex == nil || tool.Value.ToolUseId == nil || tool.Value.Name == nil {
			return nil, false, nil
		}
		identity := protocolToolIdentity{id: *tool.Value.ToolUseId, name: *tool.Value.Name}
		p.tools[*typed.Value.ContentBlockIndex] = identity
		part := corechat.NewToolCallPart(corechat.ToolCall{ID: identity.id, Name: identity.name})
		message := corechat.NewAssistantMessage(part)
		output = &corechat.Output{Message: &message}
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		part, include, err := p.mapDelta(typed.Value)
		if err != nil || !include {
			return nil, false, err
		}
		message := corechat.NewAssistantMessage(part)
		output = &corechat.Output{Message: &message}
	case *types.ConverseStreamOutputMemberMessageStop:
		output = &corechat.Output{FinishReason: mapProtocolStopReason(typed.Value.StopReason)}
		if output.FinishReason == corechat.FinishReasonOther {
			output.Metadata = &corechat.OutputMetadata{}
			if err := output.Metadata.Set(chatNativeFinishReasonKey, string(typed.Value.StopReason)); err != nil {
				return nil, false, err
			}
		}
	case *types.ConverseStreamOutputMemberMetadata:
		if typed.Value.Usage == nil {
			return nil, false, nil
		}
		response.Metadata.Usage = mapProtocolUsage(typed.Value.Usage)
	default:
		return nil, false, nil
	}

	if output != nil {
		response.Output = output
	}
	if err := response.Validate(); err != nil {
		return nil, false, fmt.Errorf("bedrock: stream response: %w", err)
	}
	return response, true, nil
}

func (p *protocolChunkAccumulator) mapDelta(delta types.ContentBlockDeltaEvent) (corechat.Part, bool, error) {
	switch value := delta.Delta.(type) {
	case *types.ContentBlockDeltaMemberText:
		if value.Value == "" {
			return corechat.Part{}, false, nil
		}
		return corechat.NewTextPart(value.Value), true, nil
	case *types.ContentBlockDeltaMemberReasoningContent:
		switch reasoning := value.Value.(type) {
		case *types.ReasoningContentBlockDeltaMemberText:
			if reasoning.Value == "" {
				return corechat.Part{}, false, nil
			}
			part := corechat.NewReasoningPart(reasoning.Value, nil)
			if err := setReasoningKind(&part, chatReasoningText); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
		case *types.ReasoningContentBlockDeltaMemberSignature:
			if reasoning.Value == "" {
				return corechat.Part{}, false, nil
			}
			part := corechat.NewReasoningPart("", []byte(reasoning.Value))
			if err := setReasoningKind(&part, chatReasoningText); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
		case *types.ReasoningContentBlockDeltaMemberRedactedContent:
			if len(reasoning.Value) == 0 {
				return corechat.Part{}, false, nil
			}
			part := corechat.NewReasoningPart("", reasoning.Value)
			if err := setReasoningKind(&part, chatReasoningRedacted); err != nil {
				return corechat.Part{}, false, err
			}
			return part, true, nil
		}
	case *types.ContentBlockDeltaMemberToolUse:
		if value.Value.Input == nil || *value.Value.Input == "" || delta.ContentBlockIndex == nil {
			return corechat.Part{}, false, nil
		}
		identity, ok := p.tools[*delta.ContentBlockIndex]
		if !ok {
			return corechat.Part{}, false, fmt.Errorf("bedrock: tool delta for unknown content block %d", *delta.ContentBlockIndex)
		}
		return corechat.NewToolCallPart(corechat.ToolCall{
			ID: identity.id, Name: identity.name, Arguments: *value.Value.Input,
		}), true, nil
	}
	return corechat.Part{}, false, nil
}
