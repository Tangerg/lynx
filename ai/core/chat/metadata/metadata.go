package metadata

import "github.com/Tangerg/lynx/ai/core/model"

// GenerationMetadata interface extends model.ResultMetadata and provides additional methods for result details.
type GenerationMetadata interface {
	model.ResultMetadata

	// FinishReason returns a string indicating the reason why the generation process finished.
	FinishReason() string

	// Usage returns an instance of the Usage interface, providing details about token usage.
	Usage() Usage
}
