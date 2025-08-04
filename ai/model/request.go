package model

// Options represents customizable options for AI model interactions.
// This marker interface allows specification of various settings and parameters
// that can influence the behavior and output of AI models.
type Options interface {
	// Model return the model name
	// Example model names:
	//   - OpenAI: "gpt-4", "gpt-3.5-turbo", "text-embedding-ada-002"
	//   - Anthropic: "claude-3-sonnet", "claude-3-haiku"
	//   - Local models: "llama-2-7b-chat", "mistral-7b-instruct"
	Model() string
}

// Request represents a request to an AI model. It encapsulates the necessary
// information required to interact with an AI model, including instructions
// or inputs and additional model options.
//
// T is the type of instructions or input required by the AI model.
// O is the type of options that must implement Options.
type Request[T any, O Options] interface {
	// Instructions returns the instructions or input required by the AI model.
	Instructions() T

	// Options returns the customizable options for AI model interactions.
	Options() O
}
