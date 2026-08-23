package operation

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"sync/atomic"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// Options carries transport-neutral metadata for one call.
type Options struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

// Result is the operation narrow waist. Public bindings restore the concrete
// acknowledgement and event types declared by the contract registry.
type Result struct {
	Value   any
	Events  iter.Seq2[any, error]
	Failure *Failure
}

// Endpoint owns operation admission and stream lifetime. Business state stays
// on the target application facade; framing stays in dispatch/transport.
type Endpoint struct {
	target       any
	invocations *invocationGroup
	ready       atomic.Bool
}

func New(target any, lifetime context.Context) (*Endpoint, error) {
	if target == nil {
		return nil, errors.New("operation: target is required")
	}
	if lifetime == nil {
		return nil, errors.New("operation: lifetime is required")
	}
	endpoint := &Endpoint{target: target, invocations: newInvocationGroup(lifetime)}
	endpoint.ready.Store(true)
	return endpoint, nil
}

func (endpoint *Endpoint) Invoke(ctx context.Context, name string, parameters any, options Options) Result {
	callCtx, release, admitted := endpoint.invocations.Attach(ctx)
	if !admitted {
		return failed(ProjectError(context.Canceled))
	}
	method, ok := contract.lookup(name)
	if !ok {
		release()
		return failed(NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", name)))
	}
	if validation := validateOptions(options); validation != nil {
		release()
		return failed(validation)
	}
	if reflect.TypeOf(parameters) != method.Meta.Params {
		release()
		return failed(NewFailure(
			protocol.ErrInvalidParams,
			fmt.Sprintf("%s parameters have type %T, want %s", name, parameters, method.Meta.Params),
		))
	}
	if err := protocol.ValidateWireTree(parameters); err != nil {
		release()
		return failed(InvalidParameters(err))
	}
	if err := callCtx.Err(); err != nil {
		release()
		return failed(ProjectError(err))
	}

	callCtx = WithRequestMeta(callCtx, options.RequestMeta)
	callCtx = withAfterEventID(callCtx, options.AfterEventID)
	result := endpoint.execute(callCtx, method, parameters)
	if result.Events == nil {
		release()
		return result
	}
	result.Events = ownStream(callCtx, result.Events, release)
	return result
}

func (endpoint *Endpoint) execute(ctx context.Context, method *Method, parameters any) Result {
	if failure := endpoint.enforceCapabilities(ctx, method.Meta, parameters); failure != nil {
		return failed(failure)
	}
	raw := method.invoke(endpoint.target, ctx, parameters)
	if raw.err != nil {
		return failed(ProjectError(raw.err))
	}
	if err := protocol.ValidateWireTree(raw.value); err != nil {
		return failed(NewFailure(protocol.ErrInternalError, "the Runtime produced an invalid response"))
	}
	return Result{Value: raw.value, Events: validateEvents(ctx, method.Meta.Event, raw.events)}
}

func validateEvents(ctx context.Context, eventType reflect.Type, events iter.Seq[any]) iter.Seq2[any, error] {
	if events == nil {
		return nil
	}
	return func(yield func(any, error) bool) {
		for event := range events {
			if reflect.TypeOf(event) != eventType {
				yield(nil, NewFailure(protocol.ErrInternalError, "the Runtime produced an event with an invalid type"))
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				yield(nil, NewFailure(protocol.ErrInternalError, "the Runtime produced an invalid event"))
				return
			}
			if !allowsEvent(ctx, event) {
				continue
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

func allowsEvent(ctx context.Context, event any) bool {
	runEvent, ok := event.(protocol.RunEvent)
	if !ok {
		return true
	}
	capabilities, ok := ClientCapabilitiesFrom(ctx)
	if !ok {
		return true
	}
	for _, excluded := range capabilities.ExcludedEphemeralEvents {
		if excluded == protocol.SuppressibleRunEventType(runEvent.Event.Type) {
			return false
		}
	}
	return true
}

func validateOptions(options Options) *Failure {
	if err := options.RequestMeta.Validate(); err != nil {
		return InvalidParameters(err)
	}
	version := options.RequestMeta.ProtocolVersion
	if version != "" && version != protocol.ProtocolVersion {
		return NewFailure(
			protocol.ErrInvalidProtocolVersion,
			fmt.Sprintf("protocolVersion %q is unsupported; expected %q", version, protocol.ProtocolVersion),
		)
	}
	return nil
}

func failed(failure *Failure) Result { return Result{Failure: failure} }

func Call[Params, Response any](
	ctx context.Context,
	endpoint *Endpoint,
	name string,
	parameters Params,
	options Options,
) (Response, error) {
	var zero Response
	result := endpoint.Invoke(ctx, name, parameters, options)
	if result.Failure != nil {
		return zero, result.Failure
	}
	response, ok := result.Value.(Response)
	if !ok {
		return zero, NewFailure(protocol.ErrInternalError, "the Runtime produced a response with an invalid type")
	}
	return response, nil
}

func CallStream[Params, Ack, Event any](
	ctx context.Context,
	endpoint *Endpoint,
	name string,
	parameters Params,
	options Options,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	result := endpoint.Invoke(ctx, name, parameters, options)
	if result.Failure != nil {
		return zero, nil, result.Failure
	}
	ack, ok := result.Value.(Ack)
	if !ok {
		return zero, nil, NewFailure(protocol.ErrInternalError, "the Runtime produced an acknowledgement with an invalid type")
	}
	return ack, restoreEventType[Event](result.Events), nil
}

func restoreEventType[Event any](events iter.Seq2[any, error]) iter.Seq2[Event, error] {
	if events == nil {
		return nil
	}
	return func(yield func(Event, error) bool) {
		for value, err := range events {
			if err != nil {
				var zero Event
				yield(zero, err)
				return
			}
			event, ok := value.(Event)
			if !ok {
				var zero Event
				yield(zero, NewFailure(protocol.ErrInternalError, "the Runtime produced an event with an invalid type"))
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

func (endpoint *Endpoint) BeginShutdown() {
	if endpoint == nil || endpoint.invocations == nil {
		return
	}
	endpoint.ready.Store(false)
	endpoint.invocations.BeginShutdown()
}

func (endpoint *Endpoint) Ready() bool {
	return endpoint != nil && endpoint.ready.Load()
}

func (endpoint *Endpoint) AwaitShutdown(ctx context.Context) error {
	if endpoint == nil || endpoint.invocations == nil {
		return nil
	}
	return endpoint.invocations.AwaitShutdown(ctx)
}
