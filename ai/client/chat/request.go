package chat

import (
	"context"
	"errors"
	"maps"
	"sync"

	"github.com/Tangerg/lynx/ai/model/chat"
)

type Request struct {
	ctx         context.Context
	fields      map[string]any
	mu          sync.RWMutex
	chatModel   chat.Model
	chatRequest *chat.Request
}

func NewRequest(ctx context.Context, session *Session) (*Request, error) {
	if ctx == nil {
		return nil, errors.New("ctx is required")
	}
	if session == nil {
		return nil, errors.New("session is required")
	}
	if session.chatModel == nil {
		return nil, errors.New("chatModel is required")
	}

	normalizedMessages, err := session.NormalizeMessages()
	if err != nil {
		return nil, err
	}

	chatRequest, err := chat.NewRequest(normalizedMessages, session.ChatOptions())
	if err != nil {
		return nil, err
	}

	params := make(map[string]any, len(session.params))
	maps.Copy(params, session.params)

	return &Request{
		ctx:         ctx,
		fields:      params,
		chatModel:   session.chatModel,
		chatRequest: chatRequest,
	}, nil
}

func (r *Request) Context() context.Context {
	if r.ctx == nil {
		return context.Background()
	}

	return r.ctx
}

func (r *Request) Set(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fields[key] = value
}

func (r *Request) SetMap(fieldsMap map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	maps.Copy(r.fields, fieldsMap)
}

func (r *Request) Get(key string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	value, exists := r.fields[key]
	return value, exists
}

func (r *Request) ChatRequest() *chat.Request {
	return r.chatRequest
}

func (r *Request) String() string {
	return ""
}
