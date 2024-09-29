package model

import "context"

// StreamingModel is a generic interface that defines a streaming operation.
// It is parameterized with two types, TReq and TRes, which represent the request
// and response types, respectively.
//
// Type Parameters:
//   - TReq: Represents the type of the request object that will be passed to the Stream method.
//   - TRes: Represents the type of the response object that will be returned by the Stream method.
//
// Methods:
//   - Stream(ctx context.Context, req TReq) (TRes, error):
//     This method initiates a streaming operation using the provided context and request object.
//     It returns a response object of type TRes and an error if the operation fails.
//
// Parameters:
//   - ctx: A context.Context object that carries deadlines, cancellation signals, and other
//     request-scoped values across API boundaries and between processes.
//   - req: An object of type TReq that contains the necessary information to perform the streaming operation.
//
// Returns:
//   - TRes: The response object resulting from the streaming operation.
//   - error: An error object that indicates if the streaming operation failed. If the operation
//     is successful, this will be nil.
type StreamingModel[TReq any, TRes any] interface {
	Stream(ctx context.Context, req TReq) (TRes, error)
}
