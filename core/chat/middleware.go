package chat

import "slices"

// CallMiddleware wraps a Model with cross-cutting call behavior. Concrete
// logging, tracing, retry, history, and safety policy belong to upper modules;
// core/chat only owns this composition vocabulary.
type CallMiddleware func(next Model) Model

// StreamMiddleware wraps the optional Streamer capability.
type StreamMiddleware func(next Streamer) Streamer

// Wrap composes call middlewares around model. The first middleware is the
// outermost wrapper. Nil entries are ignored so optional middleware can be
// supplied without a separate branch.
func Wrap(model Model, middlewares ...CallMiddleware) Model {
	return compose(model, middlewares)
}

// WrapStream composes stream middlewares around streamer using the same
// outermost-first order as Wrap.
func WrapStream(streamer Streamer, middlewares ...StreamMiddleware) Streamer {
	return compose(streamer, middlewares)
}

// compose is the one generic in the target Chat SPI: it reuses an actual
// wrapping algorithm for two concrete capabilities rather than creating a
// nominal generic model hierarchy.
func compose[T any, M ~func(T) T](endpoint T, middlewares []M) T {
	wrapped := endpoint
	for _, middleware := range slices.Backward(middlewares) {
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
	}
	return wrapped
}
