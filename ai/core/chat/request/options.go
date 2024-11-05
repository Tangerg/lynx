package request

import "github.com/Tangerg/lynx/ai/core/model"

// ChatRequestOptions  interface defines a set of methods for configuring model generation options.
type ChatRequestOptions interface {
	model.RequestOptions

	// Model returns a pointer to a string representing the name of the model to be used.
	Model() *string

	// MaxTokens returns a pointer to an int64 representing the maximum number of tokens to generate.
	MaxTokens() *int64

	// PresencePenalty returns a pointer to a float64 used to set the penalty for the presence of certain tokens in the generated text.
	PresencePenalty() *float64

	// StopSequences returns a slice of strings containing the sequences that will stop the text generation.
	StopSequences() []string

	// Temperature returns a pointer to a float64 used to set the randomness of the text generation.
	Temperature() *float64

	// TopK returns a pointer to an int64 used to set the top-k sampling parameter for text generation.
	TopK() *int64

	// TopP returns a pointer to a float64 used to set the nucleus sampling parameter for text generation.
	TopP() *float64

	// Clone returns a new instance of ChatOptions, copying the current configuration.
	Clone() ChatRequestOptions
}
