package message

type Headers map[string]any

func NewHeaders() Headers {
	return make(Headers)
}

func (h Headers) Set(key string, value any) Headers {
	h[key] = value
	return h
}
func (h Headers) Get(key string) (any, bool) {
	v, ok := h[key]
	return v, ok
}
