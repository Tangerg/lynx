package chat

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Tangerg/lynx/ai/core/model/media"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/function"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type helper struct {
	function.Support[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata]
	defaultOptions *OpenAIChatOptions
}

func (h *helper) getFinishReason(reason openai.FinishReason) chatMetadata.FinishReason {
	switch reason {
	case openai.FinishReasonStop:
		return chatMetadata.Stop

	case openai.FinishReasonLength:
		return chatMetadata.Length

	case openai.FinishReasonFunctionCall,
		openai.FinishReasonToolCalls:
		return chatMetadata.ToolCalls

	case openai.FinishReasonContentFilter:
		return chatMetadata.ContentFilter

	case openai.FinishReasonNull, "":
		return chatMetadata.Null

	default:
		return chatMetadata.Other
	}
}

func (h *helper) getRole(mt message.Type) string {
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

func (h *helper) createApiMultiContent(media []*media.Media) []openai.ChatMessagePart {
	rv := make([]openai.ChatMessagePart, 0, len(media))
	for _, m := range media {
		part := openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{},
		}
		if m.MimeType() == nil {
			part.ImageURL.URL = string(m.Data())
		} else {
			part.ImageURL.URL =
				fmt.Sprintf("data:%s;base64,%s",
					m.MimeType().String(),
					base64.StdEncoding.EncodeToString(m.Data()),
				)
		}
		rv = append(rv, part)
	}
	return rv
}

func (h *helper) createApiMessages(msgs []message.ChatMessage) []openai.ChatCompletionMessage {
	rv := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, chatMessage := range msgs {
		msg := openai.ChatCompletionMessage{
			Role:    h.getRole(chatMessage.Type()),
			Content: chatMessage.Content(),
		}
		if chatMessage.Type().IsUser() {
			userMessage := chatMessage.(*message.UserMessage)
			if len(userMessage.Media()) > 0 {
				msg.MultiContent = append(msg.MultiContent, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: userMessage.Content(),
				})
				msg.MultiContent = append(msg.MultiContent, h.createApiMultiContent(userMessage.Media())...)
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
		if chatMessage.Type().IsTool() {
			toolCallsMessage := chatMessage.(*message.ToolCallsMessage)
			for _, response := range toolCallsMessage.Responses() {
				if response.ID == "" {
					continue
				}
				rv = append(rv, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    response.Data,
					Name:       response.Name,
					ToolCallID: response.ID,
				})
			}

		} else {
			rv = append(rv, msg)
		}
	}
	return rv
}
func (h *helper) createApiRequest(prompt OpenAIChatPrompt, stream bool) *openai.ChatCompletionRequest {
	rv := &openai.ChatCompletionRequest{}

	if stream {
		rv.Stream = true
		rv.StreamOptions = &openai.StreamOptions{
			IncludeUsage: true,
		}
	}

	rv.Messages = h.createApiMessages(prompt.Instructions())

	opts := prompt.Options()
	if opts == nil {
		return rv
	}

	rv.Model = *opts.Model()

	if opts.MaxTokens() != nil {
		rv.MaxTokens = int(*opts.MaxTokens())
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
		h.RegisterFunctions(f)
	}

	return rv
}

func (h *helper) createToolCallRequests(toolCalls []openai.ToolCall) []*message.ToolCallRequest {
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

func (h *helper) createCallResponse(resp *openai.ChatCompletionResponse) OpenAIChatCompletion {
	usage := metadata.
		NewOpenAIUsage().
		IncrPromptTokens(int64(resp.Usage.PromptTokens)).
		IncrCompletionTokens(int64(resp.Usage.CompletionTokens)).
		IncrReasoningTokens(int64(resp.Usage.CompletionTokensDetails.ReasoningTokens)).IncrTotalTokens(int64(resp.Usage.CompletionTokens))

	cm := chatMetadata.
		NewChatCompletionMetadataBuilder().
		WithID(resp.ID).
		WithModel(resp.Model).
		WithUsage(usage).
		WithCreated(resp.Created).
		Build()

	builder := newChatCompletionBuilder().
		WithMetadata(cm)

	for _, choice := range resp.Choices {
		gm := metadata.
			NewOpenAIChatGenerationMetadataBuilder().
			WithFinishReason(h.getFinishReason(choice.FinishReason)).
			Build()

		var msg *message.AssistantMessage
		if len(choice.Message.ToolCalls) > 0 {
			msg = message.NewAssistantMessage(
				choice.Message.Content,
				nil,
				h.createToolCallRequests(choice.Message.ToolCalls),
			)
		} else {
			msg = message.NewAssistantMessage(choice.Message.Content, nil, nil)
		}

		gen, _ := builder.
			NewChatGenerationBuilder().
			WithMessage(msg).
			WithMetadata(gm).
			Build()

		builder.WithChatGenerations(gen)
	}

	completion, _ := builder.Build()
	return completion
}

func (h *helper) createStreamResponse(resp *openai.ChatCompletionStreamResponse) OpenAIChatCompletion {
	cmb := chatMetadata.
		NewChatCompletionMetadataBuilder().
		WithID(resp.ID).
		WithModel(resp.Model).
		WithCreated(resp.Created)

	if resp.Usage != nil {
		usage := metadata.
			NewOpenAIUsage().
			IncrPromptTokens(int64(resp.Usage.PromptTokens)).
			IncrCompletionTokens(int64(resp.Usage.CompletionTokens)).
			IncrReasoningTokens(int64(resp.Usage.CompletionTokensDetails.ReasoningTokens)).IncrTotalTokens(int64(resp.Usage.CompletionTokens))
		cmb.WithUsage(usage)
	}

	builder := newChatCompletionBuilder().
		WithMetadata(cmb.Build())

	for _, choice := range resp.Choices {
		gm := metadata.
			NewOpenAIChatGenerationMetadataBuilder().
			WithFinishReason(h.getFinishReason(choice.FinishReason)).
			Build()

		gen, _ := builder.
			NewChatGenerationBuilder().
			WithContent(choice.Delta.Content).
			WithMetadata(gm).
			Build()

		builder.WithChatGenerations(gen)
	}

	completion, _ := builder.Build()
	return completion
}

func (h *helper) merageStreamResponse(resps []openai.ChatCompletionStreamResponse) (OpenAIChatCompletion, error) {
	if len(resps) == 0 {
		return nil, errors.New("empty response")
	}
	if len(resps) == 1 {
		return h.createStreamResponse(&resps[0]), nil
	}

	lastResp := resps[len(resps)-1]

	contents := make([]*strings.Builder, len(lastResp.Choices))
	for i := range contents {
		contents[i] = &strings.Builder{}
	}

	for _, resp := range resps {
		for i, choice := range resp.Choices {
			contents[i].WriteString(choice.Delta.Content)
		}
	}

	for i, choice := range lastResp.Choices {
		choice.Delta.Content = contents[i].String()
		lastResp.Choices[i] = choice
	}

	return h.createStreamResponse(&lastResp), nil
}
