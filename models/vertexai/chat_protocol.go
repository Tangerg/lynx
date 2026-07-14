package vertexai

import (
	"errors"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/google"
)

// ChatConfig configures a Core chat adapter backed by Vertex AI.
type ChatConfig struct {
	Project        string
	Location       string
	DefaultOptions corechat.Options
}

// NewChat constructs a Core chat adapter backed by Vertex AI and Application
// Default Credentials.
func NewChat(cfg ChatConfig) (*google.Chat, error) {
	if cfg.Project == "" {
		return nil, errors.New("vertexai: Project is required")
	}
	if cfg.Location == "" {
		return nil, errors.New("vertexai: Location is required")
	}
	return google.NewChat(google.ChatConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
	})
}
