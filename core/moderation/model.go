package moderation

import "context"

// Model is the complete provider-neutral moderation SPI. Call implementations
// validate requests before I/O, reject explicit options they cannot represent,
// preserve context error identity, and return responses that pass Validate.
// Provider defaults and identity belong to provider construction and
// observability.
type Model interface {
	Call(context.Context, *Request) (*Response, error)
}

// ModelFunc adapts a function to [Model].
type ModelFunc func(context.Context, *Request) (*Response, error)

func (f ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return f(ctx, request)
}
