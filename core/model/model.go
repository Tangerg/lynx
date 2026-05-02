package model

// Model is the synchronous request-response surface for an AI model.
// It composes [CallHandler] without adding methods so any handler-shaped
// value satisfies it; the named interface exists to make code that takes
// "an AI model" self-documenting.
//
// Use Model when one full response is required before continuing — for
// example single-turn Q&A, batch embeddings, image generation, or
// classification. For incremental output use [StreamingModel] instead.
//
// Example:
//
//	type myModel struct{ /* ... */ }
//	func (m *myModel) Call(ctx context.Context, req *MyRequest) (*MyResponse, error) { ... }
//
//	var _ Model[*MyRequest, *MyResponse] = (*myModel)(nil)
type Model[Request any, Response any] interface {
	CallHandler[Request, Response]
}

// StreamingModel is the streaming counterpart of [Model]. It composes
// [StreamHandler] without adding methods, so any stream-handler-shaped
// value satisfies it.
//
// Use StreamingModel for real-time chat, long-form generation, live
// transcription / translation, or any case where reducing time-to-first-byte
// matters more than waiting for the full response.
//
// Example:
//
//	type myStreamModel struct{ /* ... */ }
//	func (m *myStreamModel) Stream(ctx context.Context, req *MyRequest) iter.Seq2[*MyChunk, error] { ... }
//
//	var _ StreamingModel[*MyRequest, *MyChunk] = (*myStreamModel)(nil)
type StreamingModel[Request any, Response any] interface {
	StreamHandler[Request, Response]
}
