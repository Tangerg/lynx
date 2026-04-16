package google

import "github.com/Tangerg/lynx/models/internal/options"

const (
	Provider = "Google"
)

const (
	OptionsKey = "lynx:ai:model:google_options"
)

func getOptionsParams[T any](
	opts interface {
		Get(key string) (any, bool)
	},
) *T {
	return options.GetParams[T](opts, OptionsKey)
}
