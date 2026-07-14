package google

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/model"
)

// ChatConfig configures the provider-neutral Core chat adapter.
type ChatConfig struct {
	APIKey         model.APIKey
	DefaultOptions corechat.Options
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
}

// Validate verifies construction-time configuration.
func (c ChatConfig) Validate() error {
	if c.Backend != genai.BackendVertexAI && c.APIKey == nil {
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
// It coexists with the frozen legacy ChatModel during workspace migration.
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
		APIKey:   cfg.APIKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: cloneProtocolOptions(cfg.DefaultOptions)}, nil
}

// Call performs one non-streaming GenerateContent request.
func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	modelName, contents, config, err := c.buildProtocolRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := c.api.ChatCompletion(ctx, modelName, contents, config)
	if err != nil {
		return nil, err
	}
	mapper := newProtocolResponseMapper()
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
		mapper := newProtocolResponseMapper()
		for response, streamErr := range c.api.ChatCompletionStream(ctx, modelName, contents, config) {
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
	return mapProtocolRequest(c.defaults, req)
}
