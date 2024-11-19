package chat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model/media"
	"github.com/Tangerg/lynx/pkg/mime"
	"github.com/sashabaranov/go-openai"
)

type converter struct{}

func (c *converter) convertFinishReason(reason openai.FinishReason) result.FinishReason {
	switch reason {
	case openai.FinishReasonStop:
		return result.Stop

	case openai.FinishReasonLength:
		return result.Length

	case openai.FinishReasonFunctionCall,
		openai.FinishReasonToolCalls:
		return result.ToolCalls

	case openai.FinishReasonContentFilter:
		return result.ContentFilter

	case openai.FinishReasonNull, "":
		return result.Null

	default:
		return result.Other
	}
}

func (c *converter) getMessageRole(mt message.Type) string {
	if mt.IsUser() {
		return openai.ChatMessageRoleUser
	}
	if mt.IsSystem() {
		return openai.ChatMessageRoleSystem
	}
	if mt.IsAssistant() {
		return openai.ChatMessageRoleAssistant
	}
	if mt.IsTool() {
		return openai.ChatMessageRoleTool
	}
	return openai.ChatMessageRoleUser
}

func (c *converter) makeApiChatMessageParts(media []*media.Media) []openai.ChatMessagePart {
	rv := make([]openai.ChatMessagePart, 0, len(media))

	for _, m := range media {
		part := openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{},
		}

		if mime.IsImage(m.MimeType()) {
			part.ImageURL.URL = fmt.Sprintf(
				"data:%s;base64,%s",
				m.MimeType().String(),
				base64.StdEncoding.EncodeToString(m.Data()),
			)
		} else {
			part.ImageURL.URL = string(m.Data())
		}

		rv = append(rv, part)
	}

	return rv
}

func (c *converter) makeApiChatCompletionMessages(msgs []message.ChatMessage) []openai.ChatCompletionMessage {
	rv := make([]openai.ChatCompletionMessage, 0, len(msgs))

	for _, chatMessage := range msgs {
		if chatMessage.Type().IsTool() {
			toolCallsMessage := chatMessage.(*message.ToolCallsMessage)
			for _, resp := range toolCallsMessage.Responses() {
				if resp.ID == "" {
					continue
				}
				rv = append(rv, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    resp.Data,
					Name:       resp.Name,
					ToolCallID: resp.ID,
				})
			}
			continue
		}

		msg := openai.ChatCompletionMessage{
			Role:    c.getMessageRole(chatMessage.Type()),
			Content: chatMessage.Content(),
		}

		if chatMessage.Type().IsUser() {
			userMessage := chatMessage.(*message.UserMessage)
			if len(userMessage.Media()) > 0 {
				msg.Content = ""
				msg.MultiContent = append(msg.MultiContent, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: userMessage.Content(),
				})
				msg.MultiContent = append(msg.MultiContent, c.makeApiChatMessageParts(userMessage.Media())...)
			}
		}

		if chatMessage.Type().IsAssistant() {
			assistantMessage := chatMessage.(*message.AssistantMessage)
			for _, toolCallRequest := range assistantMessage.ToolCalls() {
				msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
					ID:   toolCallRequest.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      toolCallRequest.Name,
						Arguments: toolCallRequest.Arguments,
					},
				})
			}
		}
		rv = append(rv, msg)
	}

	return rv
}

func (c *converter) makeApiChatCompletionRequest(req *OpenAIChatRequest, stream bool) *openai.ChatCompletionRequest {
	rv := &openai.ChatCompletionRequest{}

	if stream {
		rv.Stream = true
		rv.StreamOptions = &openai.StreamOptions{
			IncludeUsage: true,
		}
	}

	rv.Messages = c.makeApiChatCompletionMessages(req.Instructions())

	opts := req.Options()
	if opts == nil {
		return rv
	}

	rv.Model = *opts.Model()

	if opts.MaxTokens() != nil {
		rv.MaxTokens = *opts.MaxTokens()
	}

	if opts.Temperature() != nil {
		rv.Temperature = float32(*opts.Temperature())
	}

	if opts.TopP() != nil {
		rv.TopP = float32(*opts.TopP())
	}

	rv.N = opts.N()

	rv.Stop = opts.StopSequences()

	if opts.PresencePenalty() != nil {
		rv.PresencePenalty = float32(*opts.PresencePenalty())
	}

	for _, f := range opts.Functions() {
		rv.Tools = append(rv.Tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        f.Name(),
				Description: f.Description(),
				Strict:      true,
				Parameters:  json.RawMessage(f.InputTypeSchema()),
			},
		})
	}

	return rv
}

func (c *converter) makeMessageToolCallRequests(toolCalls []openai.ToolCall) []*message.ToolCallRequest {
	rv := make([]*message.ToolCallRequest, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		rv = append(rv, &message.ToolCallRequest{
			ID:        toolCall.ID,
			Type:      "function",
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		})
	}
	return rv
}

func (c *converter) makeOpenAIChatResponse(resp *openai.ChatCompletionResponse) *OpenAIChatResponse {
	usage := NewOpenAIUsage().
		IncrPromptTokens(int64(resp.Usage.PromptTokens)).
		IncrCompletionTokens(int64(resp.Usage.CompletionTokens)).
		IncrReasoningTokens(int64(resp.Usage.CompletionTokensDetails.ReasoningTokens)).
		IncrTotalTokens(int64(resp.Usage.CompletionTokens))

	responseMetadata := response.
		NewChatResponseMetadataBuilder().
		WithID(resp.ID).
		WithModel(resp.Model).
		WithUsage(usage).
		WithCreated(resp.Created).
		Build()

	builder := newOpenAIChatResponseBuilder().
		WithMetadata(responseMetadata)

	for _, choice := range resp.Choices {
		resultMetadata := NewOpenAIChatResultMetadataBuilder().
			WithFinishReason(c.convertFinishReason(choice.FinishReason)).
			Build()

		assistantMessage := message.NewAssistantMessage(
			choice.Message.Content,
			nil,
			c.makeMessageToolCallRequests(choice.Message.ToolCalls),
		)

		chatResult, _ := builder.
			NewChatResultBuilder().
			WithMessage(assistantMessage).
			WithMetadata(resultMetadata).
			Build()

		builder.WithChatResults(chatResult)
	}

	rv, _ := builder.Build()
	return rv
}

func (c *converter) makeOpenAIChatResponseByStreamChunk(resp *openai.ChatCompletionStreamResponse) *OpenAIChatResponse {
	responseMetadataBuilder := response.
		NewChatResponseMetadataBuilder().
		WithID(resp.ID).
		WithModel(resp.Model).
		WithCreated(resp.Created)

	if resp.Usage != nil {
		usage := NewOpenAIUsage().
			IncrPromptTokens(int64(resp.Usage.PromptTokens)).
			IncrCompletionTokens(int64(resp.Usage.CompletionTokens)).
			IncrReasoningTokens(int64(resp.Usage.CompletionTokensDetails.ReasoningTokens)).
			IncrTotalTokens(int64(resp.Usage.CompletionTokens))
		responseMetadataBuilder.WithUsage(usage)
	}

	builder := newOpenAIChatResponseBuilder().
		WithMetadata(responseMetadataBuilder.Build())

	for _, choice := range resp.Choices {
		resultMetada := NewOpenAIChatResultMetadataBuilder().
			WithFinishReason(c.convertFinishReason(choice.FinishReason)).
			Build()

		chatResult, _ := builder.
			NewChatResultBuilder().
			WithContent(choice.Delta.Content).
			WithMetadata(resultMetada).
			Build()

		builder.WithChatResults(chatResult)
	}

	rv, _ := builder.Build()
	return rv
}
