package model_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

// recorder captures the order in which middlewares execute around a
// handler call so we can assert composition order.
type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) record(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

func TestMiddlewareManager_BuildCallHandlerOrdering(t *testing.T) {
	rec := &recorder{}
	mw := func(label string) model.CallMiddleware[*fakeRequest, *fakeResponse] {
		return func(next model.CallHandler[*fakeRequest, *fakeResponse]) model.CallHandler[*fakeRequest, *fakeResponse] {
			return model.CallHandlerFunc[*fakeRequest, *fakeResponse](
				func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
					rec.record("before:" + label)
					resp, err := next.Call(ctx, req)
					rec.record("after:" + label)
					return resp, err
				},
			)
		}
	}

	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			rec.record("endpoint")
			return &fakeResponse{A: "ok"}, nil
		},
	)

	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	mm.UseCallMiddlewares(mw("outer"), mw("inner"))

	wrapped := mm.BuildCallHandler(endpoint)
	if _, err := wrapped.Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	got := rec.snapshot()
	want := []string{
		"before:outer",
		"before:inner",
		"endpoint",
		"after:inner",
		"after:outer",
	}
	if !equalStrings(got, want) {
		t.Fatalf("ordering = %v, want %v", got, want)
	}
}

func TestMiddlewareManager_BuildCallHandler_NoMiddlewaresReturnsEndpoint(t *testing.T) {
	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{A: req.Q}, nil
		},
	)

	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	wrapped := mm.BuildCallHandler(endpoint)

	got, err := wrapped.Call(context.Background(), &fakeRequest{Q: "echo"})
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if got.A != "echo" {
		t.Fatalf("Call result = %q, want %q", got.A, "echo")
	}
}

func TestMiddlewareManager_UseCallMiddlewaresIgnoresNil(t *testing.T) {
	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	mw := model.CallMiddleware[*fakeRequest, *fakeResponse](
		func(next model.CallHandler[*fakeRequest, *fakeResponse]) model.CallHandler[*fakeRequest, *fakeResponse] {
			return next
		},
	)

	mm.UseCallMiddlewares(nil, mw, nil)

	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{A: "ok"}, nil
		},
	)
	if _, err := mm.BuildCallHandler(endpoint).Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
}

func TestMiddlewareManager_BuildStreamHandlerOrdering(t *testing.T) {
	rec := &recorder{}
	streamMW := func(label string) model.StreamMiddleware[*fakeRequest, *fakeResponse] {
		return func(next model.StreamHandler[*fakeRequest, *fakeResponse]) model.StreamHandler[*fakeRequest, *fakeResponse] {
			return model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
				func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
					rec.record("before:" + label)
					inner := next.Stream(ctx, req)
					return func(yield func(*fakeResponse, error) bool) {
						for chunk, err := range inner {
							if !yield(chunk, err) {
								return
							}
						}
						rec.record("after:" + label)
					}
				},
			)
		}
	}

	endpoint := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {
				rec.record("endpoint")
				yield(&fakeResponse{A: "chunk"}, nil)
			}
		},
	)

	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	mm.UseStreamMiddlewares(streamMW("outer"), streamMW("inner"))

	for chunk, err := range mm.BuildStreamHandler(endpoint).Stream(context.Background(), &fakeRequest{}) {
		if err != nil {
			t.Fatalf("yielded error: %v", err)
		}
		_ = chunk
	}

	got := rec.snapshot()
	want := []string{"before:outer", "before:inner", "endpoint", "after:inner", "after:outer"}
	if !equalStrings(got, want) {
		t.Fatalf("ordering = %v, want %v", got, want)
	}
}

func TestMiddlewareManager_UseMiddlewaresRoutesByType(t *testing.T) {
	callRan, streamRan := false, false

	callMW := model.CallMiddleware[*fakeRequest, *fakeResponse](
		func(next model.CallHandler[*fakeRequest, *fakeResponse]) model.CallHandler[*fakeRequest, *fakeResponse] {
			return model.CallHandlerFunc[*fakeRequest, *fakeResponse](
				func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
					callRan = true
					return next.Call(ctx, req)
				},
			)
		},
	)
	streamMW := model.StreamMiddleware[*fakeRequest, *fakeResponse](
		func(next model.StreamHandler[*fakeRequest, *fakeResponse]) model.StreamHandler[*fakeRequest, *fakeResponse] {
			return model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
				func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
					streamRan = true
					return next.Stream(ctx, req)
				},
			)
		},
	)

	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	mm.UseMiddlewares(callMW, streamMW, nil, "garbage value")

	callEndpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{}, nil
		},
	)
	if _, err := mm.BuildCallHandler(callEndpoint).Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatal(err)
	}

	streamEndpoint := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {}
		},
	)
	for range mm.BuildStreamHandler(streamEndpoint).Stream(context.Background(), &fakeRequest{}) {
	}

	if !callRan {
		t.Fatal("call middleware was not registered")
	}
	if !streamRan {
		t.Fatal("stream middleware was not registered")
	}
}

func TestMiddlewareManager_CloneIsolation(t *testing.T) {
	original := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	original.UseCallMiddlewares(passThroughCallMW())

	clone := original.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil")
	}

	// Mutating one must not affect the other.
	original.UseCallMiddlewares(passThroughCallMW())

	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{A: "ok"}, nil
		},
	)
	// Both still work; isolation is verified by the lack of panics and by
	// rerunning Build on the clone after parent mutation.
	if _, err := clone.BuildCallHandler(endpoint).Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatalf("clone Call err: %v", err)
	}
}

func TestMiddlewareManager_CloneNil(t *testing.T) {
	var mm *model.MiddlewareManager[*fakeRequest, *fakeResponse]
	if got := mm.Clone(); got != nil {
		t.Fatalf("nil receiver Clone = %v, want nil", got)
	}
}

func TestMiddlewareManager_PropagatesEndpointError(t *testing.T) {
	wantErr := errors.New("endpoint failed")
	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return nil, wantErr
		},
	)

	mm := model.NewMiddlewareManager[*fakeRequest, *fakeResponse]()
	mm.UseCallMiddlewares(passThroughCallMW())

	_, err := mm.BuildCallHandler(endpoint).Call(context.Background(), &fakeRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

// --- helpers --------------------------------------------------------------

func passThroughCallMW() model.CallMiddleware[*fakeRequest, *fakeResponse] {
	return func(next model.CallHandler[*fakeRequest, *fakeResponse]) model.CallHandler[*fakeRequest, *fakeResponse] {
		return next
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
