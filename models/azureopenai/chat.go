package azureopenai

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type ChatModelConfig struct {
	// APIKey is the Azure OpenAI resource key. Leave empty when
	// authenticating with [azure.WithTokenCredential] passed
	// through RequestOptions.
	APIKey string

	// Endpoint is the resource URL — e.g.
	// "https://my-resource.openai.azure.com". Required.
	Endpoint string

	// APIVersion targets a dated REST version. Empty falls back to
	// [DefaultAPIVersion].
	APIVersion string

	DefaultOptions *chat.Options

	// RequestOptions reach the underlying openai-go client. Use
	// [azure.WithTokenCredential] here for Azure AD auth.
	RequestOptions []option.RequestOption
}

func (c ChatModelConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("azureopenai: Endpoint is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("azureopenai: DefaultOptions is required")
	}
	return nil
}

// NewChatModel returns an [openai.ChatModel] pointed at Azure OpenAI.
// [chat.Options].Model should be the Azure deployment id, not the
// underlying model name.
func NewChatModel(cfg ChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}

// buildAzureRequestOptions wires azure.WithEndpoint + azure.WithAPIKey
// into the RequestOptions slice. When APIKey is empty the config still
// needs to satisfy openai.*ModelConfig's non-empty APIKey check, so a
// placeholder is synthesized; the actual auth header (Bearer or API-Key) is set by
// the Azure middleware in RequestOptions.
func buildAzureRequestOptions(apiKey string, endpoint, apiVersion string, extra []option.RequestOption) (string, []option.RequestOption) {
	version := cmp.Or(apiVersion, DefaultAPIVersion)
	opts := []option.RequestOption{azure.WithEndpoint(endpoint, version)}
	if apiKey != "" {
		opts = append(opts, azure.WithAPIKey(apiKey))
	}
	opts = append(opts, extra...)

	keyParam := apiKey
	if keyParam == "" {
		keyParam = "azure-ad"
	}
	return keyParam, opts
}
