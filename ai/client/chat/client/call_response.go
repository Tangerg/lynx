package client

import (
	"context"
	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type CallResponse interface {
	Text(ctx context.Context) (string, error)
	Response(ctx context.Context) (*response.ChatResponse, error)
}
