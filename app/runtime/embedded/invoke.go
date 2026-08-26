package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
)

func (r *Runtime) invoke[Request, Response any](
	ctx context.Context,
	name operation.Name,
	request Request,
	options operation.Options,
) (Response, error) {
	var zero Response
	endpoint, err := r.endpoint()
	if err != nil {
		return zero, err
	}
	return endpoint.Call[Request, Response](ctx, name, request, options)
}

func (r *Runtime) invokeAck[Request any](
	ctx context.Context,
	name operation.Name,
	request Request,
	options operation.Options,
) error {
	_, err := r.invoke[Request, struct{}](ctx, name, request, options)
	return err
}

func (r *Runtime) invokeStream[Request, Ack, Event any](
	ctx context.Context,
	name operation.Name,
	request Request,
	options operation.Options,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	endpoint, err := r.endpoint()
	if err != nil {
		return zero, nil, err
	}
	return endpoint.CallStream[Request, Ack, Event](ctx, name, request, options)
}
