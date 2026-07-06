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

func TestMiddlewareChain_BuildCallHandlerOrdering(t *testing.T) {
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

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithCall(mw("outer"), mw("inner"))

	wrapped := chain.BuildCallHandler(endpoint)
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

func TestMiddlewareChain_BuildCallHandler_NoMiddlewaresReturnsEndpoint(t *testing.T) {
	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{A: req.Q}, nil
		},
	)

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]()
	wrapped := chain.BuildCallHandler(endpoint)

	got, err := wrapped.Call(context.Background(), &fakeRequest{Q: "echo"})
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if got.A != "echo" {
		t.Fatalf("Call result = %q, want %q", got.A, "echo")
	}
}

func TestMiddlewareChain_WithCallIgnoresNil(t *testing.T) {
	mw := model.CallMiddleware[*fakeRequest, *fakeResponse](
		func(next model.CallHandler[*fakeRequest, *fakeResponse]) model.CallHandler[*fakeRequest, *fakeResponse] {
			return next
		},
	)

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithCall(nil, mw, nil)

	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{A: "ok"}, nil
		},
	)
	if _, err := chain.BuildCallHandler(endpoint).Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
}

func TestMiddlewareChain_BuildStreamHandlerOrdering(t *testing.T) {
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

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithStream(streamMW("outer"), streamMW("inner"))

	for chunk, err := range chain.BuildStreamHandler(endpoint).Stream(context.Background(), &fakeRequest{}) {
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

func TestMiddlewareChain_CallAndStreamAreTypedSeparately(t *testing.T) {
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

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithCall(callMW).
		WithStream(streamMW)

	callEndpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return &fakeResponse{}, nil
		},
	)
	if _, err := chain.BuildCallHandler(callEndpoint).Call(context.Background(), &fakeRequest{}); err != nil {
		t.Fatal(err)
	}

	streamEndpoint := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {}
		},
	)
	for range chain.BuildStreamHandler(streamEndpoint).Stream(context.Background(), &fakeRequest{}) {
	}

	if !callRan {
		t.Fatal("call middleware was not registered")
	}
	if !streamRan {
		t.Fatal("stream middleware was not registered")
	}
}

func TestMiddlewareChain_CloneIsolation(t *testing.T) {
	original := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithCall(passThroughCallMW())

	clone := original.Clone()

	// Mutating one must not affect the other.
	original = original.WithCall(passThroughCallMW(), passThroughCallMW())
	if got := len(clone.CallMiddlewares()); got != 1 {
		t.Fatalf("clone call middlewares = %d, want 1", got)
	}
	if got := len(original.CallMiddlewares()); got != 2 {
		t.Fatalf("original call middlewares = %d, want 2", got)
	}

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

func TestMiddlewareChain_PropagatesEndpointError(t *testing.T) {
	wantErr := errors.New("endpoint failed")
	endpoint := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return nil, wantErr
		},
	)

	chain := model.NewMiddlewareChain[*fakeRequest, *fakeResponse]().
		WithCall(passThroughCallMW())

	_, err := chain.BuildCallHandler(endpoint).Call(context.Background(), &fakeRequest{})
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
