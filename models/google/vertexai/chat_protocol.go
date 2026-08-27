package vertexai

import (
	"context"
	"errors"
	"iter"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type ChatConfig struct {
	Client         ClientConfig
	DefaultOptions corechat.Options
}

func (c ChatConfig) Validate() error {
	return c.Client.validateOptions(c.DefaultOptions)
}

func (c ChatConfig) protocol() protocol.ChatConfig {
	return protocol.ChatConfig{
		Provider:       protocolProvider,
		Backend:        genai.BackendVertexAI,
		Project:        c.Client.Project,
		Location:       c.Client.Location,
		DefaultOptions: c.DefaultOptions,
		BaseURL:        c.Client.BaseURL,
		HTTPClient:     c.Client.HTTPClient,
	}
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

type Chat struct{ protocol *protocol.Chat }

func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: adapter}, nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("vertexai: nil Chat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("vertexai: nil Chat")) }
	}
	return c.protocol.Stream(ctx, req)
}
