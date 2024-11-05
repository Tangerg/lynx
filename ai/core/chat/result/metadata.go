package result

import "github.com/Tangerg/lynx/ai/core/model"

type ChatResultMetadata interface {
	model.ResultMetadata

	// FinishReason returns a string indicating the reason why the generation process finished.
	FinishReason() FinishReason
}
