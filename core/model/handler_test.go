package model_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

// fakeRequest / fakeResponse keep tests focused on the handler plumbing
// without dragging in any concrete modality types.
type (
	fakeRequest  struct{ Q string }
	fakeResponse struct{ A string }
)

func TestCallHandlerFunc_DelegatesToFunction(t *testing.T) {
	want := &fakeResponse{A: "answer"}
	handler := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			if req.Q != "question" {
				t.Fatalf("unexpected request payload: %q", req.Q)
			}
			return want, nil
		},
	)

	got, err := handler.Call(context.Background(), &fakeRequest{Q: "question"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Call returned %v, want %v", got, want)
	}
}

func TestCallHandlerFunc_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	handler := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return nil, wantErr
		},
	)

	_, err := handler.Call(context.Background(), &fakeRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Call err = %v, want %v", err, wantErr)
	}
}

func TestCallHandlerFunc_HonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	handler := model.CallHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) (*fakeResponse, error) {
			return nil, ctx.Err()
		},
	)

	_, err := handler.Call(ctx, &fakeRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call err = %v, want context.Canceled", err)
	}
}

func TestStreamHandlerFunc_YieldsChunks(t *testing.T) {
	chunks := []*fakeResponse{{A: "1"}, {A: "2"}, {A: "3"}}
	handler := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {
				for _, c := range chunks {
					if !yield(c, nil) {
						return
					}
				}
			}
		},
	)

	var got []*fakeResponse
	for chunk, err := range handler.Stream(context.Background(), &fakeRequest{}) {
		if err != nil {
			t.Fatalf("Stream yielded error: %v", err)
		}
		got = append(got, chunk)
	}

	if len(got) != len(chunks) {
		t.Fatalf("got %d chunks, want %d", len(got), len(chunks))
	}
	for i, c := range got {
		if c.A != chunks[i].A {
			t.Fatalf("chunk[%d] = %q, want %q", i, c.A, chunks[i].A)
		}
	}
}

func TestStreamHandlerFunc_EarlyTermination(t *testing.T) {
	produced := 0
	handler := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {
				for range 100 {
					produced++
					if !yield(&fakeResponse{A: "chunk"}, nil) {
						return
					}
				}
			}
		},
	)

	for chunk := range handler.Stream(context.Background(), &fakeRequest{}) {
		_ = chunk
		break
	}

	// Caller broke out after the first chunk; the producer must observe
	// that and stop, not run all 100 iterations.
	if produced > 1 {
		t.Fatalf("producer ran %d times after early break, want 1", produced)
	}
}

func TestStreamHandlerFunc_PropagatesError(t *testing.T) {
	wantErr := errors.New("stream failed")
	handler := model.StreamHandlerFunc[*fakeRequest, *fakeResponse](
		func(ctx context.Context, req *fakeRequest) iter.Seq2[*fakeResponse, error] {
			return func(yield func(*fakeResponse, error) bool) {
				yield(nil, wantErr)
			}
		},
	)

	gotErr := error(nil)
	for _, err := range handler.Stream(context.Background(), &fakeRequest{}) {
		gotErr = err
	}
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("stream err = %v, want %v", gotErr, wantErr)
	}
}
