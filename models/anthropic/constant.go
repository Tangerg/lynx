package anthropic

import "github.com/Tangerg/lynx/models/internal/options"

const (
	Provider = "Anthropic"
)

const (
	OptionsKey = "lynx:ai:model:anthropic_options"
)

func getOptionsParams[T any](
	opts interface {
		Get(key string) (any, bool)
	},
) *T {
	return options.GetParams[T](opts, OptionsKey)
}
