package vertexai

import (
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/google"
)

type ChatModelConfig struct {
	// Project is the GCP project id hosting the model.
	Project string

	// Location is the GCP region — see Location* constants.
	Location string

	DefaultOptions *chat.Options
}

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("vertexai: config must not be nil")
	}
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("vertexai: DefaultOptions is required")
	}
	return nil
}

// NewChatModel returns a [google.ChatModel] backed by Vertex AI.
// Authentication flows through Application Default Credentials.
func NewChatModel(cfg *ChatModelConfig) (*google.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return google.NewChatModel(&google.ChatModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}
