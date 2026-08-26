package operation

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
)

// Registry is the single catalog of Runtime operations. It owns both the
// machine-readable method facts and the binding-neutral typed invocation.
type Registry struct {
	byName map[Name]*Method
	names  []Name
}

// Method combines immutable contract metadata with one type-erased invocation
// closure. Type erasure exists only at this private narrow waist; registrations
// and public bindings remain fully typed.
type Method struct {
	Meta   MethodMeta
	invoke func(any, context.Context, any) rawResult
}

type rawResult struct {
	value  any
	events iter.Seq[any]
	err    error
}

func newRegistry() *Registry {
	return &Registry{byName: make(map[Name]*Method)}
}

func (r *Registry) add(meta MethodMeta, invoke func(any, context.Context, any) rawResult) {
	if err := meta.Validate(); err != nil {
		panic("operation: invalid method registration: " + err.Error())
	}
	if _, exists := r.byName[meta.Name]; exists {
		panic(fmt.Sprintf("operation: method %q is registered twice", meta.Name))
	}
	r.byName[meta.Name] = &Method{Meta: cloneMethodMeta(meta), invoke: invoke}
	r.names = append(r.names, meta.Name)
}

func (r *Registry) lookup(name Name) (*Method, bool) {
	method, ok := r.byName[name]
	return method, ok
}

// Lookup returns an immutable snapshot of one method's metadata.
func (r *Registry) Lookup(name Name) (MethodMeta, bool) {
	method, ok := r.lookup(name)
	if !ok {
		return MethodMeta{}, false
	}
	return cloneMethodMeta(method.Meta), true
}

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
	meta.Materializes = slices.Clone(meta.Materializes)
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

// StreamMethods lists methods that return an acknowledgement followed by events.
func (r *Registry) StreamMethods() []string {
	var out []string
	for _, name := range r.names {
		if r.byName[name].Meta.Kind == KindStream {
			out = append(out, name.String())
		}
	}
	return out
}

// Query registers one unary read operation and derives its complete method
// semantics from the typed handler.
func (r *Registry) Query[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) {
	meta.Kind = KindUnary
	meta.Operation = OperationQuery
	meta.Idempotency = IdempotencyNone
	meta.ReplayCursor = ReplayCursorNone
	r.registerUnary(meta, call)
}

// Command registers one unary mutation with response replay semantics.
func (r *Registry) Command[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) {
	meta.Kind = KindUnary
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayResponse
	meta.ReplayCursor = ReplayCursorNone
	r.registerUnary(meta, call)
}

func (r *Registry) registerUnary[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Response]()
	pagination, err := paginationOf(meta.Params, meta.Result)
	if err != nil {
		panic(fmt.Sprintf("operation: %s has invalid pagination shapes: %v", meta.Name, err))
	}
	meta.Pagination = pagination
	r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		value, err := call(capability, ctx, typed)
		return rawResult{value: value, err: err}
	})
}

// CommandAck registers one unary mutation whose success carries no data.
func (r *Registry) CommandAck[Capability, Params any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) error,
) {
	meta.Kind = KindUnary
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayResponse
	meta.ReplayCursor = ReplayCursorNone
	meta.Pagination = PaginationNone
	meta.Params = reflect.TypeFor[Params]()
	r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		return rawResult{value: struct{}{}, err: call(capability, ctx, typed)}
	})
}

// Subscription registers one live stream that starts from current state.
func (r *Registry) Subscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq[Event], error),
) {
	meta.ReplayCursor = ReplayCursorNone
	r.registerSubscription(meta, call)
}

func (r *Registry) registerSubscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq[Event], error),
) {
	meta.Kind = KindStream
	meta.Operation = OperationSubscription
	meta.Idempotency = IdempotencyNone
	r.registerStream(meta, call)
}

// RunSubscription registers a stream that may replay retained Run events from
// an opaque cursor. Runtime-wide invalidation subscriptions use Subscription:
// reconnecting those streams deliberately resyncs instead of replaying history.
func (r *Registry) RunSubscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq[Event], error),
) {
	meta.ReplayCursor = ReplayCursorRun
	r.registerSubscription(meta, call)
}

// RunStreamCommand registers one replay-protected Run mutation and its event
// stream as a single operation.
func (r *Registry) RunStreamCommand[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq[Event], error),
) {
	meta.Kind = KindStream
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyReplayRunStream
	meta.ReplayCursor = ReplayCursorRun
	r.registerStream(meta, call)
}

func (r *Registry) registerStream[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq[Event], error),
) {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Ack]()
	meta.Event = reflect.TypeFor[Event]()
	meta.Pagination = PaginationNone
	r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		ack, events, err := call(capability, ctx, typed)
		return rawResult{value: ack, events: eraseEventType(events), err: err}
	})
}

func capabilityAvailable(capability any) bool {
	if capability == nil {
		return false
	}
	value := reflect.ValueOf(capability)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func eraseEventType[Event any](events iter.Seq[Event]) iter.Seq[any] {
	if events == nil {
		return nil
	}
	return func(yield func(any) bool) {
		for event := range events {
			if !yield(event) {
				return
			}
		}
	}
}

// paginationOf distinguishes cursor reads from bounded collection reads using
// the actual request and response shapes.
func paginationOf(params, result reflect.Type) (PaginationKind, error) {
	if params == nil {
		return PaginationNone, errors.New("params type is required")
	}
	params = contractshape.Deref(params)
	cursor, hasCursor := contractshape.LookupField(params, "cursor")
	limit, hasLimit := contractshape.LookupField(params, "limit")
	var data, nextCursor contractshape.Field
	var hasData, hasNextCursor bool
	if result != nil {
		result = contractshape.Deref(result)
		data, hasData = contractshape.LookupField(result, "data")
		nextCursor, hasNextCursor = contractshape.LookupField(result, "nextCursor")
	}

	if !hasData && !hasNextCursor {
		if hasCursor {
			return PaginationNone, fmt.Errorf("%s has a cursor request field but its result is not a page", params)
		}
		return PaginationNone, nil
	}
	if !hasData || !hasNextCursor {
		return PaginationNone, fmt.Errorf("page results require data and nextCursor together; result=%s", result)
	}
	if hasCursor != hasLimit {
		return PaginationNone, fmt.Errorf("%s must declare cursor and limit together", params)
	}
	if data.Type.Kind() != reflect.Slice || data.Optional {
		return PaginationNone, fmt.Errorf("%s.data must be a required array", result)
	}
	if nextCursor.Type.Kind() != reflect.String || !nextCursor.Optional {
		return PaginationNone, fmt.Errorf("%s.nextCursor must be an optional string", result)
	}
	if !hasCursor {
		return PaginationNone, nil
	}
	if cursor.Type.Kind() != reflect.String || !cursor.Optional {
		return PaginationNone, fmt.Errorf("%s.cursor must be an optional string", params)
	}
	if limit.Type.Kind() != reflect.Int || !limit.Optional {
		return PaginationNone, fmt.Errorf("%s.limit must be an optional int", params)
	}
	return PaginationCursor, nil
}

// Contract returns the immutable Runtime operation catalog.
func Contract() *Registry { return contract }
