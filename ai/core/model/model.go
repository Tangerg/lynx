package model

import "context"

// Model is a generic interface that represents a model capable of processing
// requests and returning responses. It is parameterized by four types:
//   - Treq: The type of the request data.
//   - OReq: The type of the options associated with the request.
//   - TRes: The type of the response data.
//   - MRes: The type of the metadata associated with the response, which must
//     implement the ResultMetadata interface.
type Model[Treq any, OReq Options, TRes any, MRes ResultMetadata] interface {
	// Call processes a request of type Request[Treq, OReq] within the given
	// context and returns a response of type Response[TRes, MRes] or an error
	// if the processing fails.
	Call(ctx context.Context, req Request[Treq, OReq]) (Response[TRes, MRes], error)
}
