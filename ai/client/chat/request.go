package chat

import (
	"context"
	"errors"
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat/model"

	"github.com/Tangerg/lynx/ai/model/chat/request"
)

type Request struct {
	ctx         context.Context
	fields      map[string]any
	mu          sync.RWMutex
	chatModel   model.ChatModel
	chatRequest *request.ChatRequest
}

func NewRequest(ctx context.Context, options *Options) (*Request, error) {
	if ctx == nil {
		return nil, errors.New("ctx is required")
	}
	if options == nil {
		return nil, errors.New("options is required")
	}
	if options.chatModel == nil {
		return nil, errors.New("chatModel is required")
	}

	msgs, err := options.prepareMessages()
	if err != nil {
		return nil, err
	}
	opts := options.prepareChatOptions()

	chatRequest, err := request.
		NewChatRequestBuilder().
		WithMessages(msgs...).
		WithOptions(opts).
		Build()
	if err != nil {
		return nil, err
	}

	fields := make(map[string]any, len(options.middlewareParams))
	maps.Copy(fields, options.middlewareParams)
	return &Request{
		ctx:         ctx,
		fields:      fields,
		chatModel:   options.chatModel,
		chatRequest: chatRequest,
	}, nil
}

func (c *Request) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Request) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fields[key] = value
}

func (c *Request) SetMap(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	maps.Copy(c.fields, m)
}

func (c *Request) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.fields[key]
	return val, ok
}

func (c *Request) ChatRequest() *request.ChatRequest {
	return c.chatRequest
}

func (c *Request) String() string {
	return ""
}
