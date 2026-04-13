package openai

const (
	Provider = "OpenAI"
)

const (
	OptionsKey = "lynx:ai:model:openai_options"
)

func getOptionsParams[T any](
	opts interface {
		Get(key string) (any, bool)
	},
) *T {
	params := new(T)

	if extra, exist := opts.Get(OptionsKey); exist && extra != nil {
		if extraParams, ok := extra.(*T); ok {
			params = extraParams
		}
	}
	return params
}
