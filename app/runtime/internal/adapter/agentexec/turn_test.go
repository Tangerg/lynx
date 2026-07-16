package agentexec

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// stallContext is the per-turn silence watchdog: it cancels when no keepAlive
// arrives within the idle window, but never while progress keeps flowing.

func TestStallContext_CancelsOnSilence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, _, stop := stallContext(context.Background(), 30*time.Millisecond)
		defer stop()
		<-ctx.Done()
	})
}

func TestStallContext_KeepAliveDefersCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, keepAlive, stop := stallContext(context.Background(), 120*time.Millisecond)
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

func TestStallContext_StopCancels(t *testing.T) {
	ctx, _, stop := stallContext(context.Background(), time.Hour)
	stop()
	if ctx.Err() == nil {
		t.Error("stop must cancel the context")
	}
	stop() // idempotent — must not panic
}
