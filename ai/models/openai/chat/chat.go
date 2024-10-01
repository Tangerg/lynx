package chat

import (
	"context"
	"io"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/models/openai/api"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

var _ model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatPrompt = *prompt.ChatPrompt[*OpenAIChatOptions]
type OpenAIChatCompletion = *completion.ChatCompletion[*metadata.OpenAIChatGenerationMetadata]

type OpenAIChatModel struct {
	openAIApi *api.OpenAIApi
}

func (o *OpenAIChatModel) promptToCompletionRequest(prompt OpenAIChatPrompt) *openai.CompletionRequest {
	return &openai.CompletionRequest{}
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req OpenAIChatPrompt) (OpenAIChatCompletion, error) {
	creq := o.promptToCompletionRequest(req)
	stream, err := o.openAIApi.CreateCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var (
		res        OpenAIChatCompletion
		sb         strings.Builder
		builder    = completion.NewChatCompletionBuilder[*metadata.OpenAIChatGenerationMetadata]()
		genBuilder = builder.NewChatGenerationBuilder()
	)

	for {
		recv, err1 := stream.Recv()
		if err1 == io.EOF {
			break
		}
		if err1 != nil {
			gen, _ := genBuilder.Build()
			res, _ = builder.WithChatGenerations(gen).Build()
			return res, err1
		}
		streamFunc := req.Options().StreamFunc()
		if streamFunc != nil {
			err2 := streamFunc(ctx, []byte(recv.Choices[0].Text))
			if err2 != nil {
				gen, _ := genBuilder.Build()
				res, _ = builder.WithChatGenerations(gen).Build()
				return res, err2
			}
		}

		md := metadata.
			NewOpenAIChatGenerationMetadataBuilder().
			FromCompletionResponse(&recv).
			Build()
		sb.WriteString(recv.Choices[0].Text)

		genBuilder.
			WithMetadata(md).
			WithContent(sb.String())

	}

	gen, err := genBuilder.Build()
	if err != nil {
		return nil, err
	}
	return builder.WithChatGenerations(gen).Build()
}

func (o *OpenAIChatModel) Call(ctx context.Context, req OpenAIChatPrompt) (OpenAIChatCompletion, error) {
	creq := o.promptToCompletionRequest(req)
	cres, err := o.openAIApi.CreateCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	md := metadata.
		NewOpenAIChatGenerationMetadataBuilder().
		FromCompletionResponse(&cres).
		Build()

	builder := completion.NewChatCompletionBuilder[*metadata.OpenAIChatGenerationMetadata]()
	gen, err := builder.
		NewChatGenerationBuilder().
		WithContent(cres.Choices[0].Text).
		WithMetadata(md).
		Build()
	if err != nil {
		return nil, err
	}

	return builder.
		WithChatGenerations(gen).
		Build()
}
