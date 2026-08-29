package vertexai

import (
	"context"
	"errors"
	"iter"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type ChatConfig struct {
	Client         ClientConfig
	DefaultOptions corechat.Options
}

func (c ChatConfig) Validate() error {
	return c.protocol().Validate()
}

func (c ChatConfig) protocol() protocol.ChatConfig {
	return protocol.ChatConfig{
		Provider:       protocolProvider,
		Client:         c.Client.protocol(),
		DefaultOptions: c.DefaultOptions,
	}
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

type Chat protocol.Chat

func NewChat(config ChatConfig) (*Chat, error) {
	adapter, err := protocol.NewChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return (*Chat)(adapter), nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil {
		return nil, errors.New("vertexai: nil Chat")
	}
	return (*protocol.Chat)(c).Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("vertexai: nil Chat")) }
	}
	return (*protocol.Chat)(c).Stream(ctx, req)
}
