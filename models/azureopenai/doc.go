// Package azureopenai wraps Azure OpenAI Service.
//
// Azure OpenAI hosts the same OpenAI models (GPT-4, GPT-4o, GPT-5,
// embeddings, DALL·E, Whisper, gpt-image-1, tts-1...) on Azure
// infrastructure, but with three protocol differences vs OpenAI's
// public API:
//
//   - the request URL is rewritten to inject a deployment id —
//     callers send the deployment name (configured in the Azure
//     portal) in [chat.Options].Model rather than a canonical model
//     id like "gpt-4o";
//   - the resource URL is per-account
//     ("https://{resource}.openai.azure.com") and a dated
//     "api-version" query parameter is required;
//   - auth uses an "API-Key" header (rather than "Authorization:
//     Bearer ..."), or alternatively an Azure AD bearer token via
//     [azure.WithTokenCredential].
//
// Constructors:
//
//   - [NewChatModel] — /chat/completions
//   - [NewEmbeddingModel] — /embeddings
//   - [NewImageModel] — /images/generations
//   - [NewAudioTTSModel] — /audio/speech
//   - [NewAudioTranscriptionModel] — /audio/transcriptions
//
// All five delegate to the corresponding [models/openai] constructor
// after wiring [azure.WithEndpoint] and [azure.WithAPIKey] into the
// RequestOptions. Token-credential auth flows through
// [azure.WithTokenCredential] supplied by the caller in
// RequestOptions — leave APIKey nil in that case.
//
// Azure Content Safety (moderation) ships a different API shape than
// OpenAI moderation and is not exposed here.
//
// See https://learn.microsoft.com/azure/ai-services/openai/ for the
// reference and
// https://learn.microsoft.com/azure/ai-services/openai/reference#rest-api-versioning
// for the api-version list.
package azureopenai
