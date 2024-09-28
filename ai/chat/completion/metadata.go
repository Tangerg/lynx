package completion

import "github.com/Tangerg/lynx/ai/model"

type Usage interface {
	PromptTokens() int64
	CompletionTokens() int64
	TotalTokens() int64
}
type ResultMetadata interface {
	model.ResultMetadata
	FinishReason() string
	Usage() Usage
}
