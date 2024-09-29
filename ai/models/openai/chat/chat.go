package chat

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

var _ model.ChatModel[*OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata] = (*OpenAIChatModel)(nil)

type OpenAIChatModel struct {
}

func (o *OpenAIChatModel) Stream(ctx context.Context, req *prompt.Prompt[*OpenAIChatOptions]) (*completion.Completion[*metadata.OpenAIChatGenerationMetadata], error) {
	//TODO implement me
	panic("implement me")
}

func (o *OpenAIChatModel) Call(ctx context.Context, req *prompt.Prompt[*OpenAIChatOptions]) (*completion.Completion[*metadata.OpenAIChatGenerationMetadata], error) {
	//TODO implement me
	panic("implement me")
}
