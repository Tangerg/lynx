package embedding

import "context"

// Model is the complete provider-neutral embedding SPI. Call implementations
// validate requests before I/O, reject explicit options they cannot represent,
// preserve context error identity, and return responses that pass Validate.
// Defaults, identity, observability, batching, and dimension discovery are
// independent concerns.
type Model interface {
	// Call performs one embedding request after validating the complete batch.
	// It must not retain or mutate request, returns outputs in input order, and
	// transfers ownership of the response to the caller. Context cancellation
	// remains identifiable through errors.Is.
	Call(ctx context.Context, request *Request) (*Response, error)
}

// ModelFunc lets an ordinary function satisfy [Model] without declaring a
// named type, which is what keeps middleware and test doubles from each
// inventing their own adapter.
type ModelFunc func(context.Context, *Request) (*Response, error)

func (m ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return m(ctx, request)
}
