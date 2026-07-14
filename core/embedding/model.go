package embedding

import "context"

// Model is the complete provider-neutral embedding SPI. Defaults, identity,
// observability, batching, and dimension discovery are independent concerns.
type Model interface {
	Call(context.Context, *Request) (*Response, error)
}

// ModelFunc adapts a function to [Model].
type ModelFunc func(context.Context, *Request) (*Response, error)

func (f ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return f(ctx, request)
}

// Dimensioner is the optional capability for models whose output width is
// known without issuing an embedding request.
type Dimensioner interface {
	Dimensions(context.Context) (int, error)
}

// DimensionFunc adapts a function to [Dimensioner].
type DimensionFunc func(context.Context) (int, error)

func (f DimensionFunc) Dimensions(ctx context.Context) (int, error) {
	return f(ctx)
}
