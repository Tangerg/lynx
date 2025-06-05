package client

import (
	"context"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/pkg/stream"
)

type StreamResponse interface {
	Text(ctx context.Context) (stream.Reader[string], error)
	Response(ctx context.Context) (stream.Reader[*response.ChatResponse], error)
}
