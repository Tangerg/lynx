package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"mime"
	"slices"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

const responsesRequestExtensionKey = "openai/responses_request"

// ResponsesChat adapts OpenAI's ordered Responses API output to the minimal
// Core chat Model and Streamer capabilities.
type ResponsesChat struct {
	api      *API
	defaults corechat.Options
}

var (
	_ corechat.Model    = (*ResponsesChat)(nil)
	_ corechat.Streamer = (*ResponsesChat)(nil)
)

// NewResponsesChat constructs a Responses-API-backed Core chat adapter.
func NewResponsesChat(cfg ChatConfig) (*ResponsesChat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, RequestOptions: cfg.RequestOptions})
	if err != nil {
		return nil, err
	}
	return &ResponsesChat{api: api, defaults: cloneProtocolOptions(cfg.DefaultOptions)}, nil
}

// Call performs one non-streaming Responses API request.
func (c *ResponsesChat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildResponsesRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.ResponseNew(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapResponsesResponse(response)
}

// Stream performs one streaming Responses API request and yields ordered Core
// response deltas.
func (c *ResponsesChat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := c.buildResponsesRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		stream, err := c.api.ResponseNewStream(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		state := newResponsesStreamState()
		for stream.Next() {
			response, include, mapErr := state.addEvent(stream.Current())
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if include && !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, streamErr)
		}
	}
}

func (c *ResponsesChat) buildResponsesRequest(req *corechat.Request) (*responses.ResponseNewParams, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("openai responses: nil ResponsesChat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("openai responses: request: %w", err)
	}
	params, found, err := metadata.Decode[responses.ResponseNewParams](req.Extensions, responsesRequestExtensionKey)
	if err != nil {
		return nil, fmt.Errorf("openai responses: extension %q: %w", responsesRequestExtensionKey, err)
	}
	if !found {
		params = responses.ResponseNewParams{}
	}

	options := mergeProtocolOptions(c.defaults, req.Options)
	if options.Model == "" {
		return nil, errors.New("openai responses: model is required in defaults or request options")
	}
	if options.FrequencyPenalty != nil || options.PresencePenalty != nil || options.TopK != nil || len(options.Stop) != 0 {
		return nil, errors.New("openai responses: frequency_penalty, presence_penalty, top_k, and stop are not supported")
	}
	params.Model = shared.ResponsesModel(options.Model)
	if options.MaxTokens != nil {
		params.MaxOutputTokens = openaisdk.Int(*options.MaxTokens)
	}
	if options.Temperature != nil {
		params.Temperature = openaisdk.Float(*options.Temperature)
	}
	if options.TopP != nil {
		params.TopP = openaisdk.Float(*options.TopP)
	}
	if !slices.Contains(params.Include, responses.ResponseIncludableReasoningEncryptedContent) {
		params.Include = append(params.Include, responses.ResponseIncludableReasoningEncryptedContent)
	}

	items, err := mapResponsesInput(req.Messages)
	if err != nil {
		return nil, err
	}
	params.Input.OfInputItemList = items
	params.Tools, err = mapResponsesTools(req.Tools)
	if err != nil {
		return nil, err
	}
	return &params, nil
}

func mapResponsesTools(definitions []corechat.ToolDefinition) ([]responses.ToolUnionParam, error) {
	tools := make([]responses.ToolUnionParam, 0, len(definitions))
	for index := range definitions {
		var schema map[string]any
		if err := json.Unmarshal(definitions[index].InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("openai responses: tools[%d].input_schema: %w", index, err)
		}
		tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name: definitions[index].Name, Description: openaisdk.String(definitions[index].Description), Parameters: schema, Strict: openaisdk.Bool(true),
		}})
	}
	return tools, nil
}

func mapResponsesInput(messages []corechat.Message) (responses.ResponseInputParam, error) {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for messageIndex := range messages {
		mapped, err := mapResponsesMessage(messageIndex, messages[messageIndex])
		if err != nil {
			return nil, fmt.Errorf("openai responses: messages[%d]: %w", messageIndex, err)
		}
		items = append(items, mapped...)
	}
	return items, nil
}

func mapResponsesMessage(messageIndex int, message corechat.Message) ([]responses.ResponseInputItemUnionParam, error) {
	switch message.Role {
	case corechat.RoleSystem:
		return []responses.ResponseInputItemUnionParam{responsesEasyMessage(responses.EasyInputMessageRoleSystem, message.Text())}, nil
	case corechat.RoleUser:
		content, err := mapResponsesUserContent(message.Parts)
		if err != nil {
			return nil, err
		}
		return []responses.ResponseInputItemUnionParam{{OfMessage: &responses.EasyInputMessageParam{
			Role:    responses.EasyInputMessageRoleUser,
			Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: content},
		}}}, nil
	case corechat.RoleAssistant:
		return mapResponsesAssistantItems(messageIndex, message.Parts), nil
	case corechat.RoleTool:
		return mapResponsesToolResults(message.Parts), nil
	default:
		return nil, fmt.Errorf("unsupported role %q", message.Role)
	}
}

func responsesEasyMessage(role responses.EasyInputMessageRole, text string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
		Role: role, Content: responses.EasyInputMessageContentUnionParam{OfString: openaisdk.String(text)},
	}}
}

func mapResponsesUserContent(parts []corechat.Part) (responses.ResponseInputMessageContentListParam, error) {
	content := make(responses.ResponseInputMessageContentListParam, 0, len(parts))
	for index := range parts {
		part := parts[index]
		switch part.Kind {
		case corechat.PartText:
			content = append(content, responses.ResponseInputContentUnionParam{OfInputText: &responses.ResponseInputTextParam{Text: part.Text}})
		case corechat.PartMedia:
			mapped, err := mapResponsesMedia(part.Media)
			if err != nil {
				return nil, fmt.Errorf("parts[%d]: %w", index, err)
			}
			content = append(content, mapped)
		}
	}
	return content, nil
}

func mapResponsesMedia(value *media.Media) (responses.ResponseInputContentUnionParam, error) {
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil {
		return responses.ResponseInputContentUnionParam{}, err
	}
	if strings.HasPrefix(mediaType, "image/") {
		image := &responses.ResponseInputImageParam{Detail: responses.ResponseInputImageDetailAuto}
		if value.Source.Kind == media.SourceReference {
			reference, referenceErr := value.Reference()
			if referenceErr != nil {
				return responses.ResponseInputContentUnionParam{}, referenceErr
			}
			image.FileID = openaisdk.String(reference)
		} else {
			location, locationErr := mediaLocation(value)
			if locationErr != nil {
				return responses.ResponseInputContentUnionParam{}, locationErr
			}
			image.ImageURL = openaisdk.String(location)
		}
		return responses.ResponseInputContentUnionParam{OfInputImage: image}, nil
	}

	file := &responses.ResponseInputFileParam{Filename: openaisdk.String(value.Name)}
	switch value.Source.Kind {
	case media.SourceReference:
		reference, referenceErr := value.Reference()
		if referenceErr != nil {
			return responses.ResponseInputContentUnionParam{}, referenceErr
		}
		file.FileID = openaisdk.String(reference)
	case media.SourceURI:
		uri, uriErr := value.URI()
		if uriErr != nil {
			return responses.ResponseInputContentUnionParam{}, uriErr
		}
		file.FileURL = openaisdk.String(uri)
	case media.SourceBytes:
		data, dataErr := value.Bytes()
		if dataErr != nil {
			return responses.ResponseInputContentUnionParam{}, dataErr
		}
		file.FileData = openaisdk.String(base64.StdEncoding.EncodeToString(data))
	default:
		return responses.ResponseInputContentUnionParam{}, media.ErrInvalidSource
	}
	return responses.ResponseInputContentUnionParam{OfInputFile: file}, nil
}

func mapResponsesAssistantItems(messageIndex int, parts []corechat.Part) []responses.ResponseInputItemUnionParam {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(parts))
	for partIndex := range parts {
		part := parts[partIndex]
		switch part.Kind {
		case corechat.PartText:
			items = append(items, responsesEasyMessage(responses.EasyInputMessageRoleAssistant, part.Text))
		case corechat.PartReasoning:
			if len(part.Signature) == 0 {
				continue
			}
			item := &responses.ResponseReasoningItemParam{
				ID: fmt.Sprintf("rs_lynx_%d_%d", messageIndex, partIndex), EncryptedContent: openaisdk.String(string(part.Signature)),
			}
			if part.Text != "" {
				item.Summary = []responses.ResponseReasoningItemSummaryParam{{Text: part.Text}}
			}
			items = append(items, responses.ResponseInputItemUnionParam{OfReasoning: item})
		case corechat.PartToolCall:
			items = append(items, responses.ResponseInputItemUnionParam{OfFunctionCall: &responses.ResponseFunctionToolCallParam{
				CallID: part.ToolCall.ID, Name: part.ToolCall.Name, Arguments: part.ToolCall.Arguments,
			}})
		}
	}
	return items
}

func mapResponsesToolResults(parts []corechat.Part) []responses.ResponseInputItemUnionParam {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(parts))
	for index := range parts {
		result := parts[index].ToolResult
		items = append(items, responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: result.ID,
			Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: openaisdk.String(result.Result)},
		}})
	}
	return items
}

func mapResponsesResponse(response *responses.Response) (*corechat.Response, error) {
	if response == nil {
		return nil, errors.New("openai responses: nil response")
	}
	parts, hasToolCalls, err := responsesOutputParts(response.Output)
	if err != nil {
		return nil, err
	}
	choice := corechat.Choice{Index: 0, FinishReason: responsesFinishReason(response, hasToolCalls)}
	if len(parts) != 0 {
		message := corechat.NewAssistantMessage(parts...)
		choice.Message = &message
	}
	result := &corechat.Response{
		ID: response.ID, Model: string(response.Model), Choices: []corechat.Choice{choice}, Usage: responsesUsage(response.Usage),
	}
	if response.CreatedAt != 0 {
		if err := result.SetExtension(responseCreatedKey, int64(response.CreatedAt)); err != nil {
			return nil, err
		}
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("openai responses: response: %w", err)
	}
	return result, nil
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
			if text != "" || reasoning.EncryptedContent != "" {
				parts = append(parts, corechat.NewReasoningPart(text, []byte(reasoning.EncryptedContent)))
			}
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

func (s *responsesStreamState) addEvent(event responses.ResponseStreamEventUnion) (*corechat.Response, bool, error) {
	switch typed := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		s.responseID = typed.Response.ID
		s.model = string(typed.Response.Model)
		return nil, false, nil
	case responses.ResponseOutputItemAddedEvent:
		if typed.Item.Type != "function_call" {
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
		s.tools[call.ID] = responsesToolIdentity{id: id, name: call.Name}
		return s.deltaResponse(corechat.NewToolCallPart(corechat.ToolCall{ID: id, Name: call.Name}))
	case responses.ResponseTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return s.deltaResponse(corechat.NewTextPart(typed.Delta))
	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		identity, ok := s.tools[typed.ItemID]
		if !ok {
			return nil, false, fmt.Errorf("openai responses: arguments delta for unknown item %q", typed.ItemID)
		}
		return s.deltaResponse(corechat.NewToolCallPart(corechat.ToolCall{ID: identity.id, Name: identity.name, Arguments: typed.Delta}))
	case responses.ResponseReasoningTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return s.deltaResponse(corechat.NewReasoningPart(typed.Delta, nil))
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		if typed.Delta == "" {
			return nil, false, nil
		}
		return s.deltaResponse(corechat.NewReasoningPart(typed.Delta, nil))
	case responses.ResponseOutputItemDoneEvent:
		if typed.Item.Type != "reasoning" {
			return nil, false, nil
		}
		reasoning := typed.Item.AsReasoning()
		if reasoning.EncryptedContent == "" {
			return nil, false, nil
		}
		return s.deltaResponse(corechat.NewReasoningPart("", []byte(reasoning.EncryptedContent)))
	case responses.ResponseCompletedEvent:
		hasToolCall := slices.ContainsFunc(typed.Response.Output, func(item responses.ResponseOutputItemUnion) bool {
			return item.Type == "function_call"
		})
		response := &corechat.Response{
			ID: s.responseID, Model: s.model,
			Choices: []corechat.Choice{{Index: 0, FinishReason: responsesFinishReason(&typed.Response, hasToolCall)}},
			Usage:   responsesUsage(typed.Response.Usage),
		}
		if err := response.Validate(); err != nil {
			return nil, false, err
		}
		return response, true, nil
	default:
		return nil, false, nil
	}
}

func (s *responsesStreamState) deltaResponse(part corechat.Part) (*corechat.Response, bool, error) {
	message := corechat.NewAssistantMessage(part)
	response := &corechat.Response{
		ID: s.responseID, Model: s.model,
		Choices: []corechat.Choice{{Index: 0, Message: &message}},
	}
	if err := response.Validate(); err != nil {
		return nil, false, err
	}
	return response, true, nil
}
