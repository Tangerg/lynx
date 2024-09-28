package message

type Headers interface {
	Get(key string) (any, bool)
	Set(key string, value any) Headers
}

func NewSimpleHeaders() Headers {
	return make(SimpleHeaders)
}

type SimpleHeaders map[string]any

func (s SimpleHeaders) Get(key string) (any, bool) {
	v, ok := s[key]
	return v, ok
}

func (s SimpleHeaders) Set(key string, value any) Headers {
	s[key] = value
	return s
}
