package dispatch

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// Registry is the one place a method exists.
//
// Before it there were four tables keyed by method name — the name constants,
// the handler map, the replay-protected set, and a per-handler capability check —
// so adding a method meant remembering four edits and a reviewer had to diff four
// files to see what a method actually was. A registration states all of it once,
// and the dispatcher routes straight off this table; there is no second one to
// fall out of step with.
//
// It is a package-level value built from method EXPRESSIONS (func(*Dispatcher, …))
// rather than bound methods, so it needs no Runtime to exist. That is what lets a
// build-time tool read the contract — names, kinds, retry semantics, capability
// rules — without standing up a runtime to ask.
type Registry struct {
	byName map[string]*Method
	names  []string
}

// Method is one registered method: its metadata plus the pipeline that decodes,
// invokes, and encodes it.
type Method struct {
	Meta MethodMeta

	// handle runs the whole request: decode + constraints, capability gate, the
	// typed call, and the reply. Built by the query, command, and subscription
	// registration factories so no
	// registration can assemble the steps in the wrong order or skip one.
	handle func(*Dispatcher, context.Context, *transport.Request) HandleResult
}

func newRegistry() *Registry {
	return &Registry{byName: make(map[string]*Method)}
}

func (r *Registry) add(meta MethodMeta, handle func(*Dispatcher, context.Context, *transport.Request) HandleResult) {
	if err := meta.validate(); err != nil {
		panic("dispatch: invalid method registration: " + err.Error())
	}
	if _, exists := r.byName[meta.Name]; exists {
		panic(fmt.Sprintf("dispatch: method %q is registered twice", meta.Name))
	}
	r.byName[meta.Name] = &Method{Meta: cloneMethodMeta(meta), handle: handle}
	r.names = append(r.names, meta.Name)
}

// Lookup returns the registered method, or false for an unknown name.
func (r *Registry) Lookup(name string) (*Method, bool) {
	method, ok := r.lookup(name)
	if !ok {
		return nil, false
	}
	snapshot := *method
	snapshot.Meta = cloneMethodMeta(method.Meta)
	return &snapshot, true
}

// lookup is the dispatcher's allocation-free read of immutable registry state.
// Public tooling uses Lookup's defensive snapshot; routing never exposes this
// pointer outside the package.
func (r *Registry) lookup(name string) (*Method, bool) {
	method, ok := r.byName[name]
	return method, ok
}

// Names lists every registered method in registration order, so generated
// artifacts and diffs are stable.
func (r *Registry) Names() []string { return slices.Clone(r.names) }

// Metas lists every method's metadata in registration order.
func (r *Registry) Metas() []MethodMeta {
	out := make([]MethodMeta, 0, len(r.names))
	for _, name := range r.names {
		out = append(out, cloneMethodMeta(r.byName[name].Meta))
	}
	return out
}

func cloneMethodMeta(meta MethodMeta) MethodMeta {
	meta.Errors = slices.Clone(meta.Errors)
	meta.CapabilityRules = cloneCapabilityRules(meta.CapabilityRules)
	return meta
}

func cloneCapabilityRules(rules []CapabilityRule) []CapabilityRule {
	out := slices.Clone(rules)
	for index := range out {
		out[index].When = slices.Clone(out[index].When)
		out[index].Requires = slices.Clone(out[index].Requires)
	}
	return out
}

// StreamMethods lists the methods whose response body is this call's own event
// stream — the machine-readable set a client needs so it never has to hardcode
// method names to know which calls stream (API.md §9).
func (r *Registry) StreamMethods() []string {
	var out []string
	for _, name := range r.names {
		if r.byName[name].Meta.Kind == KindStream {
			out = append(out, name)
		}
	}
	return out
}

// Query registers a read-only method that answers with one JSON-RPC result.
//
// The pipeline is fixed here, once: decode the params (and their declared
// constraints), refuse the call if a required feature is off, invoke, encode. A
// registration supplies only what is method-specific.
func Query[Params, Result any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Result, error),
) {
	meta.Kind = KindUnary
	meta.Operation = OperationQuery
	meta.Idempotency = IdempotencyNone
	registerUnary(registry, meta, call)
}

// Command registers a replay-protected method that may change state or cause an
// external effect and returns its canonical result.
func Command[Params, Result any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Result, error),
) {
	meta.Kind = KindUnary
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayResponse
	registerUnary(registry, meta, call)
}

func registerUnary[Params, Result any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Result, error),
) {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Result]()
	pagination, err := paginationOf(meta.Params, meta.Result)
	if err != nil {
		panic(fmt.Sprintf("dispatch: %s has invalid pagination shapes: %v", meta.Name, err))
	}
	meta.Pagination = pagination
	registry.add(meta, func(d *Dispatcher, ctx context.Context, msg *transport.Request) HandleResult {
		in, bad := decode[Params](msg)
		if bad != nil {
			return responseError(msg.ID, bad)
		}
		if bad := d.enforceCapabilities(ctx, meta, msg.Params); bad != nil {
			return responseError(msg.ID, bad)
		}
		out, err := call(d, ctx, in)
		return reply(msg, out, err)
	})
}

// paginationOf recognizes the protocol's one cursor-page shape from the
// actual request and result types. A partial resemblance is rejected: publishing
// cursor without a continuation, or a continuation without a cursor input, would
// make a generated client either loop incorrectly or truncate silently.
func paginationOf(params, result reflect.Type) (PaginationKind, error) {
	if params == nil {
		return PaginationNone, errors.New("params type is required")
	}
	params = protocol.Deref(params)
	cursor, hasCursor := protocol.LookupWireField(params, "cursor")
	limit, hasLimit := protocol.LookupWireField(params, "limit")
	var data, nextCursor protocol.WireField
	var hasData, hasNextCursor bool
	if result != nil {
		result = protocol.Deref(result)
		data, hasData = protocol.LookupWireField(result, "data")
		nextCursor, hasNextCursor = protocol.LookupWireField(result, "nextCursor")
	}

	if !hasData && !hasNextCursor {
		if hasCursor {
			return PaginationNone, fmt.Errorf(
				"%s has a cursor request field but its result is not a cursor page", params,
			)
		}
		return PaginationNone, nil
	}
	if !hasData || !hasNextCursor || !hasCursor || !hasLimit {
		return PaginationNone, fmt.Errorf(
			"cursor pagination requires request cursor/limit and result data/nextCursor; params=%s result=%s",
			params,
			result,
		)
	}
	if cursor.Type.Kind() != reflect.String || !cursor.Optional {
		return PaginationNone, fmt.Errorf("%s.cursor must be an optional string", params)
	}
	if limit.Type.Kind() != reflect.Int || !limit.Optional {
		return PaginationNone, fmt.Errorf("%s.limit must be an optional int", params)
	}
	if data.Type.Kind() != reflect.Slice || data.Optional {
		return PaginationNone, fmt.Errorf("%s.data must be a required array", result)
	}
	if nextCursor.Type.Kind() != reflect.String || !nextCursor.Optional {
		return PaginationNone, fmt.Errorf("%s.nextCursor must be an optional string", result)
	}
	return PaginationCursor, nil
}

// CommandAck registers a replay-protected command whose success carries no data.
// The reply is an empty object rather than null so a client reads every response
// the same way.
func CommandAck[Params any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) error,
) {
	meta.Kind = KindUnary
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayResponse
	meta.Params = reflect.TypeFor[Params]()
	registry.add(meta, func(d *Dispatcher, ctx context.Context, msg *transport.Request) HandleResult {
		in, bad := decode[Params](msg)
		if bad != nil {
			return responseError(msg.ID, bad)
		}
		if bad := d.enforceCapabilities(ctx, meta, msg.Params); bad != nil {
			return responseError(msg.ID, bad)
		}
		return replyDone(msg, call(d, ctx, in))
	})
}

// Subscription registers a read-only method whose response body remains open as
// this call's event stream.
//
// framer is the per-request event encoder. It stays a registration argument
// because which events a client may receive is a delivery policy (declared
// events, ephemeral opt-out, replay ids) that differs per stream — the run stream
// carries replayable ids, the workspace stream deliberately does not.
func Subscription[Params, Ack, Event any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Ack, iter.Seq[Event], error),
	framer func(context.Context) func(Event) (StreamFrame, bool),
) {
	meta.Kind = KindStream
	meta.Operation = OperationSubscription
	meta.Idempotency = IdempotencyNone
	registerStream(registry, meta, call, framer)
}

// RunStreamCommand registers a replay-protected command that opens a run stream.
// A same-key retry returns the original ack and re-attaches to that run instead of
// executing the command again.
func RunStreamCommand[Params, Ack, Event any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Ack, iter.Seq[Event], error),
	framer func(context.Context) func(Event) (StreamFrame, bool),
) {
	meta.Kind = KindStream
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayRunStream
	registerStream(registry, meta, call, framer)
}

func registerStream[Params, Ack, Event any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Ack, iter.Seq[Event], error),
	framer func(context.Context) func(Event) (StreamFrame, bool),
) {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Ack]()
	meta.Event = reflect.TypeFor[Event]()
	registry.add(meta, func(d *Dispatcher, ctx context.Context, msg *transport.Request) HandleResult {
		in, bad := decode[Params](msg)
		if bad != nil {
			return responseError(msg.ID, bad)
		}
		if bad := d.enforceCapabilities(ctx, meta, msg.Params); bad != nil {
			return responseError(msg.ID, bad)
		}
		out, events, err := call(d, ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return streamingResult(msg.ID, out, adaptStream(events, framer(ctx)))
	})
}

// runEventFramer is the framer every run-event stream uses.
func runEventFramer(ctx context.Context) func(protocol.RunEvent) (StreamFrame, bool) {
	return runEventToFrameFor(ctx)
}

// runtimeEventFramer frames the change-signal stream. Its events carry no replay id
// and no client opt-out (§7.1): re-subscribing and refetching IS the recovery path,
// so there is nothing per-request to decide.
func runtimeEventFramer(context.Context) func(protocol.RuntimeEvent) (StreamFrame, bool) {
	return runtimeEventToFrame
}

// dispatchRequest routes the request to its registered method.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	method, ok := contract.lookup(msg.Method)
	if !ok {
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
	return method.handle(d, ctx, msg)
}

// Contract exposes the registered method surface to build-time tooling. The
// generator reads it instead of a hand-kept list, which is what makes "the
// artifacts match the dispatcher" true by construction rather than by review.
func Contract() *Registry { return contract }

// WireShapes exposes the registered union / constraint / state-key contract to
// build-time tooling.
func WireShapes() *Shapes { return shapes }
