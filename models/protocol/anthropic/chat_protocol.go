package anthropic

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	protocolProvider           = "anthropic"
	protocolDefaultMaxTokens   = 4096
	protocolMaximumTemperature = 1
)

// ChatConfig configures an Anthropic Messages adapter. Defaults are copied
// during construction; callers may select the model per request.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
	Headers        http.Header
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("anthropic: API key is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("anthropic: default options: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Anthropic's Messages protocol and is also the reusable
// protocol base for provider packages exposing a compatible endpoint.
type Chat struct {
	api      *api
	defaults corechat.Options
	dialect  Dialect
}

func NewChat(config ChatConfig) (*Chat, error) {
	return newChat(config, Dialect{
		Provider: protocolProvider, MaxTemperature: protocolMaximumTemperature,
		RejectTopK: true, RejectTopP: true, NativeJSONSchema: true,
	})
}

func NewCompatibleChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	return newChat(config, dialect)
}

func newChat(config ChatConfig, dialect Dialect) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if dialect.Provider == "" || strings.TrimSpace(dialect.Provider) != dialect.Provider || strings.Contains(dialect.Provider, "/") {
		return nil, errors.New("anthropic: dialect provider is required, must not contain '/', and must not have surrounding whitespace")
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

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := c.buildProtocolRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.chatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	mapped, err := mapProtocolMessage(response, c.dialect.Provider)
	if err != nil {
		return nil, err
	}
	return mapped, nil
}

func (c *Chat) CountMessageInputTokens(ctx context.Context, req *corechat.Request) (int64, error) {
	params, err := c.buildProtocolRequest(req)
	if err != nil {
		return 0, err
	}
	countParams, err := projectMessageInputTokenCount(params)
	if err != nil {
		return 0, err
	}
	response, err := c.api.countTokens(ctx, countParams)
	if err != nil {
		return 0, err
	}
	return response.InputTokens, nil
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
		stream := c.api.chatCompletionStream(ctx, params)
		if stream == nil {
			yield(nil, errors.New("anthropic: nil stream"))
			return
		}
		defer stream.Close()

		state := newProtocolStreamState(c.dialect.Provider)
		for stream.Next() {
			event := stream.Current()
			response, include, mapErr := state.mapEvent(event)
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if include && !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, c.api.wrapError(streamErr))
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
	params, err := mapProtocolRequest(c.defaults, req, c.dialect)
	if err != nil {
		return nil, err
	}
	return params, nil
}
