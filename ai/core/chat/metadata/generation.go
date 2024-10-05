package metadata

import "github.com/Tangerg/lynx/ai/core/model"

// ChatGenerationMetadata interface extends model.ResultMetadata and provides additional methods for result details.
type ChatGenerationMetadata interface {
	model.ResultMetadata

	// FinishReason returns a string indicating the reason why the generation process finished.
	FinishReason() FinishReason
}
