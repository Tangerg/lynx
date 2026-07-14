package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/openai"
)

// ChatConfig configures the Core chat adapter for Azure OpenAI.
type ChatConfig struct {
	APIKey         string
	Endpoint       string
	APIVersion     string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

// NewChat constructs a Core chat adapter for Azure OpenAI. APIKey may be
// empty when RequestOptions provide Azure AD authentication.
func NewChat(cfg ChatConfig) (*openai.Chat, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("azureopenai: Endpoint is required")
	}
	apiKey, requestOptions := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewChat(openai.ChatConfig{APIKey: apiKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: requestOptions})
}
