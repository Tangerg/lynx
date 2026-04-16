package options

func GetParams[T any](
	opts interface {
		Get(key string) (any, bool)
	},
	key string,
) *T {
	params := new(T)

	if extra, exist := opts.Get(key); exist && extra != nil {
		if extraParams, ok := extra.(*T); ok {
			params = extraParams
		}
	}
	return params
}
