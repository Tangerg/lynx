// Package bedrock wraps AWS Bedrock Runtime.
//
// Bedrock is a model-aggregation gateway — a single endpoint that
// fronts foundation models from Anthropic, Meta, Mistral, Amazon
// Titan / Nova, Cohere, AI21, Stability and others. [NewChat]
// uses the unified Converse / ConverseStream API which speaks a
// provider-agnostic message shape; [NewEmbeddingModel] targets the native
// InvokeModel contracts for Titan Text Embeddings V1/V2 and Cohere Embed
// V3/V4. Provider-only embedding controls use [EmbeddingRequestOptions] under
// [EmbeddingRequestExtensionKey].
//
// Model selection is via the upstream model id (e.g.
// "anthropic.claude-3-5-sonnet-20241022-v2:0",
// "amazon.titan-embed-text-v2:0", "meta.llama3-1-70b-instruct-v1:0",
// "us.anthropic.claude-sonnet-4-20250514-v1:0"). Bedrock supports
// regional and cross-region inference-profile IDs.
//
// AWS auth is handled by the standard aws-sdk-go-v2 chain (env vars,
// shared config, IRSA, instance role); no custom APIKey is required.
//
// See https://docs.aws.amazon.com/bedrock/ for the full reference.
package bedrock
