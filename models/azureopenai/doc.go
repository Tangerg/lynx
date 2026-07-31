// Package azureopenai adapts Azure OpenAI's current OpenAI-compatible v1 API
// to the Core model interfaces.
//
// BaseURL is the complete Azure OpenAI v1 base URL, for example
// "https://RESOURCE.openai.azure.com/openai/v1/". Model names are Azure
// deployment names. Authentication uses the API key through the standard
// OpenAI client authentication path accepted by Azure's v1 endpoint.
//
// Only the current v1 endpoint shape is modeled; no dated api-version is
// required.
package azureopenai
