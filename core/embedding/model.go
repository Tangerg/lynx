package embedding

import "context"

// Model is the complete provider-neutral embedding SPI. Call implementations
// validate requests before I/O, reject explicit options they cannot represent,
// preserve context error identity, and return responses that pass Validate.
// Defaults, identity, observability, batching, and dimension discovery are
// independent concerns.
type Model interface {
	Call(context.Context, *Request) (*Response, error)
}

// ModelFunc adapts a function to [Model].
type ModelFunc func(context.Context, *Request) (*Response, error)

func (f ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return f(ctx, request)
}
