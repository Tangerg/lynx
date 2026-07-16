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

func TestStreamingModel_IdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := streamingModel{
			streamer: chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
				return func(yield func(*chat.Response, error) bool) {
					<-ctx.Done()
					yield(nil, ctx.Err())
				}
			}),
			idleTimeout: 30 * time.Millisecond,
		}

		if _, err := model.Call(context.Background(), &chat.Request{}); !errors.Is(err, errModelStreamIdleTimeout) {
			t.Fatalf("Call error = %v, want model stream idle timeout", err)
		}
	})
}

func TestStreamingModel_CompletionTimeoutCancelRace(t *testing.T) {
	t.Run("completion wins", func(t *testing.T) {
		response, err := chat.NewResponse(chat.Choice{
			Index:        0,
			Message:      messagePointer(chat.NewAssistantMessage(chat.NewTextPart("done"))),
			FinishReason: chat.FinishReasonStop,
		})
		if err != nil {
			t.Fatal(err)
		}
		model := streamingModel{
			streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
				return func(yield func(*chat.Response, error) bool) { yield(response, nil) }
			}),
			idleTimeout: time.Hour,
		}

		got, err := model.Call(context.Background(), &chat.Request{})
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if got.Text() != "done" {
			t.Fatalf("text = %q, want done", got.Text())
		}
	})

	t.Run("timeout wins", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			model := streamingModel{
				streamer: chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						<-ctx.Done()
						yield(nil, ctx.Err())
					}
				}),
				idleTimeout: time.Millisecond,
			}
			if _, err := model.Call(context.Background(), &chat.Request{}); !errors.Is(err, errModelStreamIdleTimeout) {
				t.Fatalf("Call error = %v, want model stream idle timeout", err)
			}
		})
	})

	t.Run("cancel wins", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			model := streamingModel{
				streamer: chat.StreamerFunc(func(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
					return func(yield func(*chat.Response, error) bool) {
						<-ctx.Done()
						yield(nil, ctx.Err())
					}
				}),
				idleTimeout: time.Hour,
			}
			if _, err := model.Call(ctx, &chat.Request{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("Call error = %v, want context canceled", err)
			} else if errors.Is(err, errModelStreamIdleTimeout) {
				t.Fatalf("Call error = %v, cancellation misreported as model idle", err)
			}
		})
	})
}

func messagePointer(message chat.Message) *chat.Message { return &message }
