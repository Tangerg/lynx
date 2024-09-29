package chat

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type OpenaiChatFlux struct {
}

func (f *OpenaiChatFlux) Write(ctx context.Context, data []byte) error {
	//TODO implement me
	panic("implement me")
}

func (f *OpenaiChatFlux) Process(ctx context.Context, data []byte) (*completion.Completion[*metadata.GenerationMetadata], error) {
	//TODO implement me
	panic("implement me")
}

func (f *OpenaiChatFlux) Read(ctx context.Context, t *completion.Completion[*metadata.GenerationMetadata]) error {

	//TODO implement me
	panic("implement me")
}
