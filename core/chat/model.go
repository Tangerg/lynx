package chat

import "context"

// Model is the minimal synchronous chat capability. Implementations must
// validate req before provider I/O and return a provider-neutral Response.
//
// Streaming, default configuration, and provider identity are independent
// concerns and deliberately are not methods of Model.
type Model interface {
	Call(ctx context.Context, req *Request) (*Response, error)
}
