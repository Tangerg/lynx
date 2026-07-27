package agentexec

import (
	"context"
	"errors"
	"iter"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/lynx/core/chat"
)

// modelStreamContext is the per-model-stream silence watchdog: it cancels when no
// valid provider chunk arrives within the idle window, but never while progress
// keeps flowing.

func TestModelStreamContext_CancelsOnSilence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, _, stop := modelStreamContext(context.Background(), 30*time.Millisecond)
		defer stop()
		<-ctx.Done()
		if !errors.Is(context.Cause(ctx), errModelStreamIdleTimeout) {
			t.Fatalf("cause = %v, want model stream idle timeout", context.Cause(ctx))
		}
	})
}

func TestModelStreamContext_KeepAliveDefersCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, keepAlive, stop := modelStreamContext(context.Background(), 120*time.Millisecond)
		defer stop()

		// Advance the bubble's fake clock well inside the idle window, then
		// reset the watchdog. No wall-clock scheduling participates in the test.
		for range 6 {
			timer := time.NewTimer(30 * time.Millisecond)
			<-timer.C
			keepAlive()
			if ctx.Err() != nil {
				t.Fatal("canceled despite keepAlive within the idle window")
			}
		}
	})
}

func TestModelStreamContext_StopCancels(t *testing.T) {
	ctx, _, stop := modelStreamContext(context.Background(), time.Hour)
	stop()
	if ctx.Err() == nil {
		t.Error("stop must cancel the context")
	}
	stop() // idempotent — must not panic
}

func TestModelStreamContext_PreservesParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	want, ok := parent.Deadline()
	if !ok {
		t.Fatal("parent has no deadline")
	}
	ctx, _, stop := modelStreamContext(parent, time.Minute)
	defer stop()
	got, ok := ctx.Deadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("deadline = (%v, %v), want (%v, true)", got, ok, want)
	}
}

func collectStream(streamer chat.Streamer, ctx context.Context) ([]*chat.Response, error) {
	var responses []*chat.Response
	for response, err := range streamer.Stream(ctx, &chat.Request{}) {
		if err != nil {
			return responses, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func TestStreamIdleMiddleware_IdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		streamer := streamIdleMiddleware(30 * time.Millisecond)(
			chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
				return func(yield func(*chat.Response, error) bool) {
					<-ctx.Done()
					yield(nil, ctx.Err())
				}
			}),
		)

		if _, err := collectStream(streamer, context.Background()); !errors.Is(err, errModelStreamIdleTimeout) {
			t.Fatalf("Stream error = %v, want model stream idle timeout", err)
		}
	})
}

func TestStreamIdleMiddleware_CompletionTimeoutCancelRace(t *testing.T) {
	t.Run("completion wins", func(t *testing.T) {
		response, err := chat.NewResponse(chat.Choice{
			Index:        0,
			Message:      messagePointer(chat.NewAssistantMessage(chat.NewTextPart("done"))),
			FinishReason: chat.FinishReasonStop,
		})
		if err != nil {
			t.Fatal(err)
		}
		streamer := streamIdleMiddleware(time.Hour)(
			chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
				return func(yield func(*chat.Response, error) bool) { yield(response, nil) }
			}),
		)

		got, err := collectStream(streamer, context.Background())
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if len(got) != 1 || got[0].Text() != "done" {
			t.Fatalf("responses = %#v, want one done response", got)
		}
	})

	t.Run("timeout wins", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			streamer := streamIdleMiddleware(time.Millisecond)(
				chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						<-ctx.Done()
						yield(nil, ctx.Err())
					}
				}),
			)
			if _, err := collectStream(streamer, context.Background()); !errors.Is(err, errModelStreamIdleTimeout) {
				t.Fatalf("Stream error = %v, want model stream idle timeout", err)
			}
		})
	})

	t.Run("cancel wins", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			streamer := streamIdleMiddleware(time.Hour)(
				chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						<-ctx.Done()
						yield(nil, ctx.Err())
					}
				}),
			)
			if _, err := collectStream(streamer, ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("Stream error = %v, want context canceled", err)
			} else if errors.Is(err, errModelStreamIdleTimeout) {
				t.Fatalf("Stream error = %v, cancellation misreported as model idle", err)
			}
		})
	})
}

func messagePointer(message chat.Message) *chat.Message { return &message }
