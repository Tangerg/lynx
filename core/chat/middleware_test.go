package chat_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func TestModelFuncDelegates(t *testing.T) {
	wantErr := errors.New("failed")
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, wantErr
	})
	if _, err := model.Call(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Fatalf("Call error = %v, want %v", err, wantErr)
	}
}

func TestStreamerFuncDelegates(t *testing.T) {
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			yield(&chat.Response{ID: "chunk"}, nil)
		}
	})
	for response, err := range streamer.Stream(context.Background(), nil) {
		if err != nil || response.ID != "chunk" {
			t.Fatalf("Stream yielded %#v, %v", response, err)
		}
		return
	}
	t.Fatal("Stream did not yield")
}

func TestWrapCallOrderingAndNil(t *testing.T) {
	var order []string
	middleware := func(name string) chat.CallMiddleware {
		return func(next chat.Model) chat.Model {
			return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				order = append(order, "before:"+name)
				response, err := next.Call(ctx, request)
				order = append(order, "after:"+name)
				return response, err
			})
		}
	}
	endpoint := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		order = append(order, "endpoint")
		return &chat.Response{}, nil
	})

	wrapped := chat.Wrap(endpoint, middleware("outer"), nil, middleware("inner"))
	if _, err := wrapped.Call(context.Background(), nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := []string{"before:outer", "before:inner", "endpoint", "after:inner", "after:outer"}
	if !slices.Equal(order, want) {
		t.Fatalf("call order = %v, want %v", order, want)
	}
}

func TestWrapStreamOrdering(t *testing.T) {
	var order []string
	middleware := func(name string) chat.StreamMiddleware {
		return func(next chat.Streamer) chat.Streamer {
			return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
				order = append(order, "before:"+name)
				inner := next.Stream(ctx, request)
				return func(yield func(*chat.Response, error) bool) {
					for response, err := range inner {
						if !yield(response, err) {
							return
						}
					}
					order = append(order, "after:"+name)
				}
			})
		}
	}
	endpoint := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			order = append(order, "endpoint")
			yield(&chat.Response{}, nil)
		}
	})

	for range chat.WrapStream(endpoint, middleware("outer"), middleware("inner")).Stream(context.Background(), nil) {
	}
	want := []string{"before:outer", "before:inner", "endpoint", "after:inner", "after:outer"}
	if !slices.Equal(order, want) {
		t.Fatalf("stream order = %v, want %v", order, want)
	}
}

func TestWrapBuildIsIndependentOfInputSliceMutation(t *testing.T) {
	var got string
	set := func(value string) chat.CallMiddleware {
		return func(next chat.Model) chat.Model {
			return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				got = value
				return next.Call(ctx, request)
			})
		}
	}
	middlewares := []chat.CallMiddleware{set("original")}
	wrapped := chat.Wrap(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return &chat.Response{}, nil
	}), middlewares...)
	middlewares[0] = set("mutated")

	if _, err := wrapped.Call(context.Background(), nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "original" {
		t.Fatalf("middleware = %q, want original", got)
	}
}

func TestWrapWithNoMiddlewareReturnsEndpoint(t *testing.T) {
	endpoint := &callOnlyModel{}
	if got := chat.Wrap(endpoint); got != endpoint {
		t.Fatalf("Wrap without middleware returned %T, want original endpoint", got)
	}
}
