package openai

import (
	"errors"
	"fmt"
	"slices"

	"github.com/openai/openai-go/v3/responses"

	corechat "github.com/Tangerg/scope/core/chat"
)

type responsesToolIdentity struct {
	id   string
	name string
}

type responsesStreamState struct {
	responseID string
	model      string
	tools      map[string]responsesToolIdentity
}

func newResponsesStreamState() *responsesStreamState {
	return &responsesStreamState{tools: make(map[string]responsesToolIdentity)}
}

func (r *responsesStreamState) addEvent(event responses.ResponseStreamEventUnion) (*corechat.Response, bool, error) {
	switch typed := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		r.responseID = typed.Response.ID
		r.model = string(typed.Response.Model)
		return nil, false, nil
	case responses.ResponseOutputItemAddedEvent:
		if typed.Item.Type != responsesItemTypeFunctionCall {
			return nil, false, nil
		}
		call := typed.Item.AsFunctionCall()
		id := call.CallID
		if id == "" {
			id = call.ID
		}
		if id == "" || call.Name == "" {
			return nil, false, errors.New("openai responses: stream function call lacks ID or name")
		}
		r.tools[call.ID] = responsesToolIdentity{id: id, name: call.Name}
		return r.deltaResponse(corechat.NewToolCallPart(corechat.ToolCall{ID: id, Name: call.Name}))
	case responses.ResponseTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return r.deltaResponse(corechat.NewTextPart(typed.Delta))
	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		identity, ok := r.tools[typed.ItemID]
		if !ok {
			return nil, false, fmt.Errorf("openai responses: arguments delta for unknown item %q", typed.ItemID)
		}
		return r.deltaResponse(corechat.NewToolCallPart(corechat.ToolCall{ID: identity.id, Name: identity.name, Arguments: typed.Delta}))
	case responses.ResponseReasoningTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return r.deltaResponse(corechat.NewReasoningPart(typed.Delta, nil))
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return r.deltaResponse(corechat.NewReasoningPart(typed.Delta, nil))
	case responses.ResponseOutputItemDoneEvent:
		if typed.Item.Type != responsesItemTypeReasoning {
			return nil, false, nil
		}
		reasoning := typed.Item.AsReasoning()
		signature, err := encodeResponsesReasoningFrame(reasoning.ToParam())
		if err != nil {
			return nil, false, fmt.Errorf("openai responses: stream reasoning item: %w", err)
		}
		return r.deltaResponse(corechat.NewReasoningPart("", signature))
	case responses.ResponseCompletedEvent:
		hasToolCall := slices.ContainsFunc(typed.Response.Output, func(item responses.ResponseOutputItemUnion) bool {
			return item.Type == responsesItemTypeFunctionCall
		})
		response := &corechat.Response{
			Output: &corechat.Output{FinishReason: responsesFinishReason(&typed.Response, hasToolCall)},
			Metadata: &corechat.ResponseMetadata{
				ID: r.responseID, Model: r.model, Usage: responsesUsage(typed.Response.Usage),
			},
		}
		if err := response.Metadata.Set(ResponsesResponseExtensionKey, typed.Response); err != nil {
			return nil, false, fmt.Errorf("openai responses: preserve completed response: %w", err)
		}
		if err := response.Validate(); err != nil {
			return nil, false, err
		}
		return response, true, nil
	default:
		return nil, false, nil
	}
}

func (r *responsesStreamState) deltaResponse(part corechat.Part) (*corechat.Response, bool, error) {
	message := corechat.NewAssistantMessage(part)
	response := &corechat.Response{
		Output:   &corechat.Output{Message: &message},
		Metadata: &corechat.ResponseMetadata{ID: r.responseID, Model: r.model},
	}
	if err := response.Validate(); err != nil {
		return nil, false, err
	}
	return response, true, nil
}
