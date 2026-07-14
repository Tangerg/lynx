package azureopenai

import (
	"cmp"

	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"
)

// buildAzureRequestOptions applies endpoint, API version, and authentication
// consistently across every Azure OpenAI modality. The placeholder satisfies
// the upstream client constructor when Azure AD middleware owns authentication.
func buildAzureRequestOptions(apiKey string, endpoint, apiVersion string, extra []option.RequestOption) (string, []option.RequestOption) {
	version := cmp.Or(apiVersion, DefaultAPIVersion)
	options := []option.RequestOption{azure.WithEndpoint(endpoint, version)}
	if apiKey != "" {
		options = append(options, azure.WithAPIKey(apiKey))
	}
	options = append(options, extra...)

	if apiKey == "" {
		return "azure-ad", options
	}
	return apiKey, options
}
