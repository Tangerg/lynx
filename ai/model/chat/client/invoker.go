package client

import (
	"context"
	"iter"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/model/chat"
)

type invoker struct {
	chatModel chat.Model
}

func newInvoker(chatModel chat.Model) *invoker {
	return &invoker{
		chatModel: chatModel,
	}
}

func (i *invoker) augmentLastUserMessageOutput(chatRequest *chat.Request) {
	outputFormat, ok := chatRequest.Get(OutputFormat)
	if ok {
		chatRequest.AugmentLastUserMessageText(cast.ToString(outputFormat))
	}
}

func (i *invoker) Call(ctx context.Context, chatRequest *chat.Request) (*chat.Response, error) {
	i.augmentLastUserMessageOutput(chatRequest)
	return i.chatModel.Call(ctx, chatRequest)
}

func (i *invoker) Stream(ctx context.Context, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	i.augmentLastUserMessageOutput(chatRequest)
	return i.chatModel.Stream(ctx, chatRequest)
}
