package request

import (
	"github.com/Tangerg/lynx/ai/model/model"
)

// ChatOptions represents the common options that are portable across different
// chat models.
type ChatOptions interface {
	model.Options

	// FrequencyPenalty returns the frequency penalty to use for the chat.
	// The frequency penalty reduces the likelihood of repeating the same content.
	FrequencyPenalty() *float64

	// MaxTokens returns the maximum number of tokens to use for the chat.
	// This limits the length of the generated response.
	MaxTokens() *int

	// PresencePenalty returns the presence penalty to use for the chat.
	// The presence penalty encourages the model to talk about new topics.
	PresencePenalty() *float64

	// StopSequences returns the stop sequences to use for the chat.
	// Generation will stop when any of these sequences are encountered.
	StopSequences() []string

	// Temperature returns the temperature to use for the chat.
	// Higher values make the output more random, lower values make it more deterministic.
	Temperature() *float64

	// TopK returns the top K to use for the chat.
	// Limits the model to consider only the K most likely next tokens.
	TopK() *int

	// TopP returns the top P to use for the chat.
	// Uses nucleus sampling: considers tokens with cumulative probability up to P.
	TopP() *float64

	// Clone returns a copy of this ChatOptions.
	Clone() ChatOptions
}
