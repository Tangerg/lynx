package vertexai

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

// ChatConfig configures a Core chat adapter backed by Vertex AI.
type ChatConfig struct {
	Project        string
	Location       string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("vertexai: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

type Chat struct{ protocol *protocol.Chat }

// NewChat constructs a Core chat adapter backed by Vertex AI and Application
// Default Credentials.
func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewChat(protocol.ChatConfig{
		Provider:       "vertexai",
		Backend:        genai.BackendVertexAI,
		Project:        config.Project,
		Location:       config.Location,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        config.BaseURL,
		HTTPClient:     config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: adapter}, nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("vertexai: nil Chat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("vertexai: nil Chat")) }
	}
	return c.protocol.Stream(ctx, req)
}
