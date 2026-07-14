package anthropic

import (
	"context"
	"errors"
	"fmt"
	"iter"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	corechat "github.com/Tangerg/lynx/core/chat"
)

const protocolDefaultMaxTokens int64 = 4096

// ChatConfig configures the provider-neutral Core chat adapter. Defaults are
// copied during construction; the model may instead be selected per Request.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

// Validate verifies construction-time configuration.
func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("anthropic: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("anthropic: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements the minimal Core Model and optional Streamer capabilities.
type Chat struct {
	api      *API
	defaults corechat.Options
}

// NewChat constructs a Core chat adapter.
func NewChat(cfg ChatConfig) (*Chat, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cloneProtocolOptions(cfg.DefaultOptions)}, nil
}

// Call performs one non-streaming Messages API request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildProtocolRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.ChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapProtocolMessage(response)
}

// Stream performs one streaming Messages API request and yields provider
// deltas without accumulating text or tool arguments across events.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := c.buildProtocolRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		stream := c.api.ChatCompletionStream(ctx, params)
		if stream == nil {
			yield(nil, errors.New("anthropic: nil stream"))
			return
		}
		defer stream.Close()

		state := newProtocolStreamState()
		for stream.Next() {
			response, include, mapErr := state.mapEvent(stream.Current())
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if include && !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, streamErr)
		}
	}
}

func (c *Chat) buildProtocolRequest(req *corechat.Request) (*anthropicsdk.MessageNewParams, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("anthropic: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("anthropic: request: %w", err)
	}
	return mapProtocolRequest(c.defaults, req)
}
