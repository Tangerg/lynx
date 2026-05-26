package azureopenai

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/openai"
)

type ChatModelConfig struct {
	// APIKey is the Azure OpenAI resource key. Leave nil when
	// authenticating with [azure.WithTokenCredential] passed
	// through RequestOptions.
	APIKey model.APIKey

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

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("azureopenai: config must not be nil")
	}
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
func NewChatModel(cfg *ChatModelConfig) (*openai.ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewChatModel(&openai.ChatModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &chat.ModelMetadata{Provider: Provider},
	})
}

// buildAzureRequestOptions wires azure.WithEndpoint + azure.WithAPIKey
// into the RequestOptions slice. When APIKey is nil we still need to
// satisfy openai.*ModelConfig's non-nil APIKey check, so we synthesize
// a placeholder; the actual auth header (Bearer or API-Key) is set by
// the Azure middleware in RequestOptions.
func buildAzureRequestOptions(apiKey model.APIKey, endpoint, apiVersion string, extra []option.RequestOption) (model.APIKey, []option.RequestOption) {
	version := cmp.Or(apiVersion, DefaultAPIVersion)
	opts := []option.RequestOption{azure.WithEndpoint(endpoint, version)}
	if apiKey != nil {
		opts = append(opts, azure.WithAPIKey(apiKey.Get()))
	}
	opts = append(opts, extra...)

	keyParam := apiKey
	if keyParam == nil {
		keyParam = model.NewAPIKey("azure-ad")
	}
	return keyParam, opts
}
