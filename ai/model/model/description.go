package model

// Description describes an AI model's basic characteristics. This interface
// provides a minimal approach to model identification by focusing on the
// essential information needed for model selection and management.
//
// The name serves multiple purposes:
//   - Model identification and selection in multi-model environments
//   - Logging and debugging to track which model is being used
//   - Configuration mapping to associate models with specific settings
//   - API routing to direct requests to appropriate model endpoints
//
// Example model names:
//   - OpenAI: "gpt-4", "gpt-3.5-turbo", "text-embedding-ada-002"
//   - Anthropic: "claude-3-sonnet", "claude-3-haiku"
//   - Local models: "llama-2-7b-chat", "mistral-7b-instruct"
type Description interface {
	// Name returns the unique identifier of the model. This should be a
	// stable, consistent string that uniquely identifies the model across
	// different sessions and deployments.
	//
	// The name should be unique within the model provider's namespace and
	// consistent with the model provider's official naming conventions.
	//
	// Returns:
	//   - A non-empty string uniquely identifying the model
	Name() string
}

// ChatModelDescription describes a conversational AI model that can engage
// in text-based conversations with users. These models are designed for
// interactive dialogue and can understand context, maintain conversation
// history, and generate human-like responses.
//
// Chat models are typically used for:
//   - Question answering and information retrieval
//   - Creative writing and content generation
//   - Code generation and programming assistance
//   - Reasoning and problem-solving tasks
//
// Examples include OpenAI GPT series, Anthropic Claude, and local chat
// models like Llama or Mistral.
type ChatModelDescription interface {
	Description
}

// EmbeddingModelDescription describes a text embedding model that converts
// textual input into high-dimensional numerical vector representations.
// These vectors capture semantic meaning and enable applications like
// semantic search, recommendation systems, and RAG implementations.
//
// The vector dimension is critical as it affects:
//   - Model expressiveness and semantic capture capability
//   - Storage requirements (dimension × data type size per embedding)
//   - Computational cost for similarity calculations
//   - Compatibility with vector databases and search systems
//
// Common dimension ranges:
//   - 384-512: Lightweight models (sentence-transformers)
//   - 1024-1536: Balanced models (OpenAI ada-002, BGE)
//   - 2048-3072: High-quality models (OpenAI text-embedding-3-large)
type EmbeddingModelDescription interface {
	Description

	// Dimensions returns the number of dimensions in the embedding vectors
	// produced by this model. This is a fundamental characteristic that
	// determines storage requirements, computational complexity, and
	// compatibility with downstream systems.
	//
	// Returns:
	//   - A positive integer representing the embedding vector dimension count
	//   - Must be consistent across all embeddings produced by this model
	//
	// Examples:
	//   model.Dimensions() // Returns 1536 for OpenAI text-embedding-ada-002
	//   model.Dimensions() // Returns 384 for sentence-transformers models
	Dimensions() int
}
