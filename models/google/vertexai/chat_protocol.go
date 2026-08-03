package vertexai

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	google "github.com/Tangerg/lynx/models/google/internal/protocol"
)

// ChatConfig configures a Core chat adapter backed by Vertex AI.
type ChatConfig struct {
	Project        string
	Location       string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

type Chat struct{ protocol *google.Chat }

// NewChat constructs a Core chat adapter backed by Vertex AI and Application
// Default Credentials.
func NewChat(cfg ChatConfig) (*Chat, error) {
	if cfg.Project == "" {
		return nil, errors.New("vertexai: Project is required")
	}
	if cfg.Location == "" {
		return nil, errors.New("vertexai: Location is required")
	}
	protocol, err := google.NewChat(google.ChatConfig{
		Provider:       "vertexai",
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cfg.BaseURL,
		HTTPClient:     cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: protocol}, nil
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
