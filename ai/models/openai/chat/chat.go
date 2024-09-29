package chat

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	chatModel "github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type OpenaiChatPrompt = model.Request[[]message.Message, *OpenaiChatOptions]
type OpenaiChatCompletion = model.Response[*message.AssistantMessage, *metadata.GenerationMetadata]

type OpenaiChatModel struct {
}

func (o *OpenaiChatModel) Call(ctx context.Context, req OpenaiChatPrompt) (OpenaiChatCompletion, error) {
	//TODO implement me
	panic("implement me")
}

func (o *OpenaiChatModel) Stream(ctx context.Context, req OpenaiChatPrompt, flux model.Flux[OpenaiChatCompletion]) error {
	//TODO implement me
	panic("implement me")
}

func NewOpenaiChatModel() chatModel.ChatModel[*OpenaiChatOptions, *metadata.GenerationMetadata] {
	return &OpenaiChatModel{}
}
