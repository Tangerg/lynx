package dispatch

import (
	"context"
	"fmt"
	"iter"

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
	// typed call, and the reply. Built by [Unary] / [UnaryAck] / [Stream] so no
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
	r.byName[meta.Name] = &Method{Meta: meta, handle: handle}
	r.names = append(r.names, meta.Name)
}

// Lookup returns the registered method, or false for an unknown name.
func (r *Registry) Lookup(name string) (*Method, bool) {
	method, ok := r.byName[name]
	return method, ok
}

// Names lists every registered method in registration order, so generated
// artifacts and diffs are stable.
func (r *Registry) Names() []string { return r.names }

// Metas lists every method's metadata in registration order.
func (r *Registry) Metas() []MethodMeta {
	out := make([]MethodMeta, 0, len(r.names))
	for _, name := range r.names {
		out = append(out, r.byName[name].Meta)
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

// Unary registers a method that answers with one JSON-RPC result.
//
// The pipeline is fixed here, once: decode the params (and their declared
// constraints), refuse the call if a required feature is off, invoke, encode. A
// registration supplies only what is method-specific.
func Unary[Params, Result any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Result, error),
) {
	meta.Kind = KindUnary
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

// UnaryAck registers a method whose success carries no data. The reply is an
// empty object rather than null so a client reads every response the same way.
func UnaryAck[Params any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) error,
) {
	meta.Kind = KindUnary
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

// Stream registers a method whose response body is this call's event stream.
//
// framer is the per-request event encoder. It stays a registration argument
// because which events a client may receive is a delivery policy (declared
// events, ephemeral opt-out, replay ids) that differs per stream — the run stream
// carries replayable ids, the workspace stream deliberately does not.
func Stream[Params, Ack, Event any](
	registry *Registry,
	meta MethodMeta,
	call func(*Dispatcher, context.Context, Params) (Ack, iter.Seq[Event], error),
	framer func(context.Context) func(Event) (StreamFrame, bool),
) {
	meta.Kind = KindStream
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

// workspaceEventFramer frames the non-run workspace stream. Its events carry no
// replay id and no client opt-out (AUX_API §3): the stream's own re-subscribe is
// the recovery path, so there is nothing per-request to decide.
func workspaceEventFramer(context.Context) func(protocol.WorkspaceEvent) (StreamFrame, bool) {
	return workspaceEventToFrame
}

// dispatchRequest routes the request to its registered method.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	method, ok := contract.Lookup(msg.Method)
	if !ok {
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
	return method.handle(d, ctx, msg)
}
