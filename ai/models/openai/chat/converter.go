package chat

import (
	"errors"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type converter struct{}

func (c *converter) switchFinishReason(reason openai.FinishReason) chatMetadata.FinishReason {
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

func (c *converter) switchRole(role message.Role) string {
	if role.IsUser() {
		return openai.ChatMessageRoleUser
	}
	if role.IsSystem() {
		return openai.ChatMessageRoleSystem
	}
	if role.IsAssistant() {
		return openai.ChatMessageRoleAssistant
	}
	if role.IsTool() {
		return openai.ChatMessageRoleTool
	}
	return openai.ChatMessageRoleUser
}

func (c *converter) toOpenAIApiChatCompletionRequest(prompt OpenAIChatPrompt) *openai.ChatCompletionRequest {
	rv := &openai.ChatCompletionRequest{}

	messages := prompt.Instructions()
	for _, chatMessage := range messages {
		msg := openai.ChatCompletionMessage{
			Role:    c.switchRole(chatMessage.Role()),
			Content: chatMessage.Content(),
		}

		rv.Messages = append(rv.Messages, msg)
	}
	opts := prompt.Options()

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

	return rv
}

func (c *converter) toOpenAIChatCompletionByCall(resp *openai.ChatCompletionResponse) OpenAIChatCompletion {
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

	builder := NewChatCompletionBuilder().
		WithMetadata(cm)

	for _, choice := range resp.Choices {
		gm := metadata.
			NewOpenAIChatGenerationMetadataBuilder().
			WithFinishReason(c.switchFinishReason(choice.FinishReason)).
			Build()

		gen, _ := builder.
			NewChatGenerationBuilder().
			WithContent(choice.Message.Content).
			WithMetadata(gm).
			Build()

		builder.WithChatGenerations(gen)
	}

	completion, _ := builder.Build()
	return completion
}

func (c *converter) toOpenAIChatCompletionByStream(resp *openai.ChatCompletionStreamResponse) OpenAIChatCompletion {
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

	builder := NewChatCompletionBuilder().
		WithMetadata(cmb.Build())

	for _, choice := range resp.Choices {
		gm := metadata.
			NewOpenAIChatGenerationMetadataBuilder().
			WithFinishReason(c.switchFinishReason(choice.FinishReason)).
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

func (c *converter) combineOpenAIChatCompletion(resps []openai.ChatCompletionStreamResponse) (OpenAIChatCompletion, error) {
	if len(resps) == 0 {
		return nil, errors.New("empty response")
	}
	if len(resps) == 1 {
		return c.toOpenAIChatCompletionByStream(&resps[0]), nil
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

	return c.toOpenAIChatCompletionByStream(&lastResp), nil
}
