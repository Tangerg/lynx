package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
)

func invoke[Request, Response any](
	ctx context.Context,
	runtime *Runtime,
	name string,
	request Request,
	options operation.Options,
) (Response, error) {
	var zero Response
	endpoint, err := runtime.endpoint()
	if err != nil {
		return zero, err
	}
	return operation.Call[Request, Response](ctx, endpoint, name, request, options)
}

func invokeAck[Request any](
	ctx context.Context,
	runtime *Runtime,
	name string,
	request Request,
	options operation.Options,
) error {
	_, err := invoke[Request, struct{}](ctx, runtime, name, request, options)
	return err
}

func invokeStream[Request, Ack, Event any](
	ctx context.Context,
	runtime *Runtime,
	name string,
	request Request,
	options operation.Options,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	endpoint, err := runtime.endpoint()
	if err != nil {
		return zero, nil, err
	}
	return operation.CallStream[Request, Ack, Event](ctx, endpoint, name, request, options)
}
