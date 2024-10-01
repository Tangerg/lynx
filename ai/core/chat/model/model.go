package model

import (
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model"
)

// Model is a generic interface that represents a model capable of processing prompts
// and generating completions. It is parameterized by two types:
// O - Represents the options for the prompt.
// M - Represents the metadata for the generation of the completion.
type Model[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	// The Model interface embeds another interface, model.Model, which is
	// parameterized with specific types:
	// *prompt.ChatPrompt[O] - A pointer to a ChatPrompt struct that uses the options type O.
	// *completion.ChatCompletion[M] - A pointer to a ChatCompletion struct that uses the metadata type M.
	model.Model[*prompt.ChatPrompt[O], *completion.ChatCompletion[M]]
}
