package operation

import (
	"context"
	"fmt"
	"iter"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// Options carries binding-neutral per-call metadata. Bindings translate their
// native representation into this value before invoking the Endpoint.
type Options struct {
	RequestMeta    protocol.RequestMeta
	IdempotencyKey string
	AfterEventID   string
}

// Result is the binding-neutral outcome of one operation. Value and Events are
// type-erased only inside Runtime; typed bindings restore the catalog's declared
// types before returning to their caller.
type Result struct {
	Value   any
	Events  iter.Seq2[any, error]
	Failure *Failure
}

// Endpoint executes the one Runtime operation catalog independently of any
// transport envelope.
type Endpoint struct {
	service     Service
	idempotency *replayStore
}

// Config supplies durable operation mechanisms. A nil IdempotencyStore selects
// a Runtime-instance-local store, useful for tests and non-durable hosts.
type Config struct {
	IdempotencyStore idempotency.Store
}

// New constructs a binding-neutral operation endpoint.
func New(service Service, config Config) *Endpoint {
	store := config.IdempotencyStore
	if store == nil {
		store = newMemoryIdempotencyStore()
	}
	return &Endpoint{service: service, idempotency: newReplayStore(store)}
}

// Invoke validates and executes the named operation through the catalog's
// capability, idempotency and safe-problem policies.
func (e *Endpoint) Invoke(ctx context.Context, name string, parameters any, options Options) Result {
	method, ok := contract.lookup(name)
	if !ok {
		return failed(NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", name)))
	}
	if err := validateOptions(options); err != nil {
		return failed(err)
	}
	if reflect.TypeOf(parameters) != method.Meta.Params {
		return failed(NewFailure(
			protocol.ErrInvalidParams,
			fmt.Sprintf("%s parameters have type %T, want %s", name, parameters, method.Meta.Params),
		))
	}
	if err := protocol.ValidateWireTree(parameters); err != nil {
		return failed(InvalidParameters(err))
	}

	ctx = WithRequestMeta(ctx, options.RequestMeta)
	ctx = withAfterEventID(ctx, options.AfterEventID)
	execute := func() Result { return e.execute(ctx, method, parameters) }
	if options.IdempotencyKey == "" || !method.Meta.Idempotency.Replays() {
		return execute()
	}
	return e.idempotency.invoke(ctx, method, parameters, options.IdempotencyKey, execute, e.service)
}

func (e *Endpoint) execute(ctx context.Context, method *Method, parameters any) Result {
	if err := e.enforceCapabilities(ctx, method.Meta, parameters); err != nil {
		return failed(err)
	}
	raw := method.invoke(e.service, ctx, parameters)
	if raw.err != nil {
		return failed(ProjectError(raw.err))
	}
	if err := protocol.ValidateWireTree(raw.value); err != nil {
		return failed(NewFailure(protocol.ErrInternalError, "the runtime produced an invalid response"))
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
				yield(nil, NewFailure(protocol.ErrInternalError, "the runtime produced an event with an invalid type"))
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				yield(nil, NewFailure(protocol.ErrInternalError, "the runtime produced an invalid event"))
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
	if err := protocol.ValidateWireTree(options.RequestMeta); err != nil {
		return InvalidParameters(err)
	}
	version := options.RequestMeta.ProtocolVersion
	if version != "" && !protocol.SupportsProtocolVersion(version) {
		return NewFailure(
			protocol.ErrInvalidProtocolVersion,
			fmt.Sprintf("protocolVersion %q is unsupported; supported range is %q through %q", version, protocol.MinProtocolVersion, protocol.ProtocolVersion),
		)
	}
	return nil
}

func failed(failure *Failure) Result { return Result{Failure: failure} }

// Call restores the typed result declared by a unary catalog entry.
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
	value, ok := result.Value.(Response)
	if !ok {
		return zero, NewFailure(protocol.ErrInternalError, "the runtime produced a response with an invalid type")
	}
	return value, nil
}
