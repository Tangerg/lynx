package model

import "context"

// Model defines a contract for executing a request and returning a response.
// It uses generic types for flexibility.
//
// Type Parameters:
//   - Req: The request type.
//   - Res: The response type.
//
// Methods:
//   - Call: Executes with a given context and request, returning a response and an error.
type Model[Req any, Res any] interface {
	Call(ctx context.Context, req Req) (Res, error)
}

// StreamingModel defines a contract for executing a streaming operation.
// It also uses generic types for flexibility.
//
// Type Parameters:
//   - Req: The request type.
//   - Res: The response type.
//
// Methods:
//   - Stream: Initiates a streaming operation with a given context and request, returning a response and an error.
type StreamingModel[Req any, Res any] interface {
	Stream(ctx context.Context, req Req) (Res, error)
}
