package prompt

import "github.com/Tangerg/lynx/ai/model"

type Options interface {
	model.Options
	Model() *string
	MaxTokens() *int64
	PresencePenalty() *float64
	StopSequences() []string
	Temperature() *float64
	TopK() *int64
	TopP() *float64
	Copy() Options
}
