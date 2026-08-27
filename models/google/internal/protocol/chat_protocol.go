package protocol

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/scope/core/chat"
)

// ChatConfig configures the provider-neutral Core chat adapter.
type ChatConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions corechat.Options
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
	HTTPClient     *http.Client
}

// Validate verifies construction-time configuration.
func (c ChatConfig) Validate() error {
	if err := validateProvider(c.Provider); err != nil {
		return fmt.Errorf("google: Provider: %w", err)
	}
	if c.Backend != genai.BackendVertexAI && c.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("google: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements the minimal Core Model and optional Streamer capabilities.
type Chat struct {
	api      *api
	defaults corechat.Options
	provider string
}

// NewChat constructs a Core chat adapter.
func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		Backend:    config.Backend,
		Project:    config.Project,
		Location:   config.Location,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: config.DefaultOptions.Clone(), provider: config.Provider}, nil
}

// Call performs one non-streaming GenerateContent request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	modelName, contents, config, err := c.buildProtocolRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.chatCompletion(ctx, modelName, contents, config)
	if err != nil {
		return nil, err
	}
	mapper := newProtocolResponseMapper(c.provider)
	return mapper.mapResponse(modelName, response)
}

// Stream performs one streaming GenerateContent request. Candidate and logical
// part offsets are retained only for the lifetime of this stream.
func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		modelName, contents, config, err := c.buildProtocolRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		mapper := newProtocolResponseMapper(c.provider)
		for response, streamErr := range c.api.chatCompletionStream(ctx, modelName, contents, config) {
			if streamErr != nil {
				yield(nil, streamErr)
				return
			}
			mapped, mapErr := mapper.mapResponse(modelName, response)
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if !yield(mapped, nil) {
				return
			}
		}
	}
}

func (c *Chat) buildProtocolRequest(req *corechat.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	if c == nil || c.api == nil {
		return "", nil, nil, errors.New("google: nil Chat")
	}
	if err := req.Validate(); err != nil {
		return "", nil, nil, fmt.Errorf("google: request: %w", err)
	}
	return mapProtocolRequest(c.provider, c.defaults, req)
}
