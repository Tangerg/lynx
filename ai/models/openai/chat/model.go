package chat

import (
	"context"
	"io"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/models/openai/api"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type OpenAIChatPrompt = *prompt.ChatPrompt[*OpenAIChatOptions]

func NewChatPromptBuilder() *prompt.ChatPromptBuilder[*OpenAIChatOptions] {
	return prompt.NewChatPromptBuilder[*OpenAIChatOptions]()
}

type OpenAIChatCompletion = *completion.ChatCompletion[*metadata.OpenAIChatGenerationMetadata]

func NewChatCompletionBuilder() *completion.ChatCompletionBuilder[*metadata.OpenAIChatGenerationMetadata] {
	return completion.NewChatCompletionBuilder[*metadata.OpenAIChatGenerationMetadata]()
}

var _ model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatModel struct {
	converter *converter
	config    *OpenAIChatModelConfig
	openAIApi *api.OpenAIApi
}

func NewOpenAIChatModel(conf OpenAIChatModelConfig) model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] {
	return &OpenAIChatModel{
		converter: &converter{},
		config:    &conf,
		openAIApi: api.NewOpenAIApi(conf.Token),
	}
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req OpenAIChatPrompt) (OpenAIChatCompletion, error) {
	creq := o.converter.toOpenAIApiChatCompletionRequest(req)
	creq.Stream = true
	creq.StreamOptions = &openai.StreamOptions{
		IncludeUsage: true,
	}

	stream, err := o.openAIApi.CreateChatCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var (
		openAIChatCompletion OpenAIChatCompletion
		recv                 openai.ChatCompletionStreamResponse
		recvs                = make([]openai.ChatCompletionStreamResponse, 0, 64)
		streamFunc           = req.Options().StreamChunkFunc()
		streamCompletionFunc = req.Options().StreamCompletionFunc()
	)

	for {
		recv, err = stream.Recv()

		if err != nil {
			if err == io.EOF {
				return o.converter.combineOpenAIChatCompletion(recvs)
			}
			return openAIChatCompletion, err
		}

		if len(recv.Choices) == 0 {
			continue
		}
		recvs = append(recvs, recv)
		openAIChatCompletion = o.converter.toOpenAIChatCompletionByStream(&recv)

		if streamFunc != nil {
			err = streamFunc(ctx, recv.Choices[0].Delta.Content)
			if err != nil {
				return openAIChatCompletion, err
			}
		}

		if streamCompletionFunc != nil {
			err = streamCompletionFunc(ctx, openAIChatCompletion)
			if err != nil {
				return openAIChatCompletion, err
			}
		}
	}
}

func (o *OpenAIChatModel) Call(ctx context.Context, req OpenAIChatPrompt) (OpenAIChatCompletion, error) {
	creq := o.converter.toOpenAIApiChatCompletionRequest(req)

	cres, err := o.openAIApi.CreateChatCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	return o.converter.toOpenAIChatCompletionByCall(&cres), nil
}
