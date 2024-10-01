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

type OpenAIPrompt = *prompt.Prompt[*OpenAIChatOptions]
type OpenAICompletion = *completion.Completion[*metadata.OpenAIChatGenerationMetadata]

type OpenAIChatModel struct {
	openAIApi *api.OpenAIApi
}

func (o *OpenAIChatModel) promptToCompletionRequest(prompt OpenAIPrompt) *openai.CompletionRequest {
	return &openai.CompletionRequest{}
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req OpenAIPrompt) (OpenAICompletion, error) {
	creq := o.promptToCompletionRequest(req)
	stream, err := o.openAIApi.CreateCompletionStream(ctx, creq)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var (
		res        OpenAICompletion
		sb         strings.Builder
		builder    = completion.NewCompletionBuilder[*metadata.OpenAIChatGenerationMetadata]()
		genBuilder = builder.NewGenerationBuilder()
	)

	for {
		recv, err1 := stream.Recv()
		if err1 == io.EOF {
			break
		}
		if err1 != nil {
			gen, _ := genBuilder.Build()
			res, _ = builder.WithGenerations(gen).Build()
			return res, err1
		}
		streamFunc := req.Options().StreamFunc()
		if streamFunc != nil {
			err2 := streamFunc(ctx, []byte(recv.Choices[0].Text))
			if err2 != nil {
				gen, _ := genBuilder.Build()
				res, _ = builder.WithGenerations(gen).Build()
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
	return builder.WithGenerations(gen).Build()
}

func (o *OpenAIChatModel) Call(ctx context.Context, req OpenAIPrompt) (OpenAICompletion, error) {
	creq := o.promptToCompletionRequest(req)
	cres, err := o.openAIApi.CreateCompletion(ctx, creq)
	if err != nil {
		return nil, err
	}

	md := metadata.
		NewOpenAIChatGenerationMetadataBuilder().
		FromCompletionResponse(&cres).
		Build()

	builder := completion.NewCompletionBuilder[*metadata.OpenAIChatGenerationMetadata]()
	gen, err := builder.
		NewGenerationBuilder().
		WithContent(cres.Choices[0].Text).
		WithMetadata(md).
		Build()
	if err != nil {
		return nil, err
	}

	return builder.
		WithGenerations(gen).
		Build()
}
