package chatclient

import (
	"context"

	"github.com/Tangerg/scope/core/chat"
)

// Generation is an immutable Client with one typed output contract bound to
// it. Call and Stream both return the complete decoded result; use
// [Client.Stream] directly when individual response chunks are required.
type Generation[T any] struct {
	client *Client
	format OutputFormat[T]
}

// Call invokes the synchronous model capability and decodes its complete
// response through the same stream decoder used by Stream.
func (g Generation[T]) Call(ctx context.Context, request *chat.Request) (T, error) {
	var zero T
	if err := g.validate(); err != nil {
		return zero, err
	}
	response, err := g.client.call(ctx, request, g.format.contract.Clone())
	return g.format.decode(once(response, err))
}

// Stream consumes the streaming model capability and returns the complete
// decoded result after the response sequence ends.
func (g Generation[T]) Stream(ctx context.Context, request *chat.Request) (T, error) {
	var zero T
	if err := g.validate(); err != nil {
		return zero, err
	}
	return g.format.decode(g.client.stream(ctx, request, g.format.contract.Clone()))
}

func (g Generation[T]) validate() error {
	if g.client == nil {
		return ErrNilClient
	}
	return g.format.validate()
}
