package ollama

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	ollamaapi "github.com/ollama/ollama/api"

	corechat "github.com/Tangerg/lynx/core/chat"
)

// ChatConfig configures the provider-neutral Core chat adapter.
type ChatConfig struct {
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

// Validate verifies construction-time configuration.
func (c ChatConfig) Validate() error {
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("ollama: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Core chat over Ollama's native /api/chat endpoint. It
// coexists with NativeChatModel while the legacy model surface is frozen.
type Chat struct {
	api      *API
	defaults corechat.Options
}

// NewChat constructs a Core chat adapter.
func NewChat(cfg ChatConfig) (*Chat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cloneProtocolOptions(cfg.DefaultOptions)}, nil
}

// Call performs one non-streaming native chat request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	apiReq, err := c.buildProtocolRequest(req, false)
	if err != nil {
		return nil, err
	}

	var (
		response ollamaapi.ChatResponse
		received bool
	)
	if err := c.api.Chat(ctx, apiReq, func(next ollamaapi.ChatResponse) error {
		response = next
		received = true
		return nil
	}); err != nil {
		return nil, err
	}
	if !received {
		return nil, errors.New("ollama: chat returned no response")
	}
	return newProtocolResponseMapper().mapResponse(apiReq.Model, response)
}

// Stream bridges Ollama's callback stream into Core's pull sequence. Returning
// false from yield aborts the HTTP stream without surfacing a cancellation
// error to the caller.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		apiReq, err := c.buildProtocolRequest(req, true)
		if err != nil {
			yield(nil, err)
			return
		}

		mapper := newProtocolResponseMapper()
		consumerStopped := false
		err = c.api.Chat(ctx, apiReq, func(chunk ollamaapi.ChatResponse) error {
			mapped, mapErr := mapper.mapResponse(apiReq.Model, chunk)
			if mapErr != nil {
				return mapErr
			}
			if !yield(mapped, nil) {
				consumerStopped = true
				return context.Canceled
			}
			return nil
		})
		if err != nil && !consumerStopped {
			yield(nil, err)
		}
	}
}

func (c *Chat) buildProtocolRequest(req *corechat.Request, stream bool) (*ollamaapi.ChatRequest, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("ollama: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	return mapProtocolRequest(c.defaults, req, stream)
}
