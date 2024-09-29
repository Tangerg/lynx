package model

import "context"

// Model is a generic interface that defines a contract for executing a call with a request and returning a response.
// It is parameterized with two types: TReq and TRes, which represent the request and response types respectively.
//
// Type Parameters:
//   - TReq: Represents the type of the request object that will be passed to the Call method.
//   - TRes: Represents the type of the response object that will be returned by the Call method.
//
// Methods:
//   - Call(ctx context.Context, req TReq) (TRes, error): This method takes a context and a request of type TReq, and returns a response of type TRes along with an error. The context is used to control the lifetime of the request, allowing for cancellation and timeout management.
type Model[TReq any, TRes any] interface {
	Call(ctx context.Context, req TReq) (TRes, error)
}
