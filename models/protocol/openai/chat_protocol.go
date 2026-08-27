package openai

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
)

const (
	// RequestExtensionKey identifies provider-owned Chat Completions fields
	// encoded as [RequestFields].
	RequestExtensionKey = "openai/request"
	// ResponseExtensionKey preserves the complete official Chat Completions
	// response after provider-neutral fields have been mapped.
	ResponseExtensionKey = "openai/response"
	// StreamChunkExtensionKey preserves each complete official Chat
	// Completions stream chunk.
	StreamChunkExtensionKey = "openai/stream_chunk"
)

// ChatConfig configures an OpenAI Chat Completions adapter. DefaultOptions
// are copied during construction; callers may select the model per request.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
	Headers        http.Header
}

// Validate verifies construction-time configuration.
func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("openai: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements OpenAI's Chat Completions protocol and is also the reusable
// protocol base for provider packages exposing a compatible endpoint.
type Chat struct {
	api      *api
	defaults corechat.Options
	dialect  Dialect
}

// NewChat constructs OpenAI's native Chat Completions adapter.
func NewChat(config ChatConfig) (*Chat, error) {
	return newChat(config, Dialect{Provider: "openai", TokenLimitField: TokenLimitMaxCompletionTokens})
}

// NewCompatibleChat constructs a Chat Completions adapter with one explicit
// provider dialect. Provider packages use this seam; application code should
// prefer the provider's own constructor.
func NewCompatibleChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	return newChat(config, dialect)
}

func newChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := dialect.Validate(); err != nil {
		return nil, fmt.Errorf("openai: dialect: %w", err)
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
		Headers:    config.Headers,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{
		api:      api,
		defaults: config.DefaultOptions.Clone(),
		dialect:  dialect,
	}, nil
}

// Call performs one non-streaming Chat Completions request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildRequest(req, false)
	if err != nil {
		return nil, err
	}
	response, err := c.api.chatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapCompletion(params, response, c.dialect)
}

// Stream performs one streaming Chat Completions request. Each yielded Core
// response represents only the current provider delta; stable tool identity is
// retained in adapter-local state.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := c.buildRequest(req, true)
		if err != nil {
			yield(nil, err)
			return
		}

		stream, err := c.api.chatCompletionStream(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		state := newOpenAIStreamState(c.dialect)
		for stream.Next() {
			response, mapErr := state.mapChunk(stream.Current())
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, c.api.wrapError(streamErr))
		}
	}
}
