package agentexec

import (
	"context"
	"testing"
	"time"
)

// stallContext is the per-turn silence watchdog: it cancels when no keepAlive
// arrives within the idle window, but never while progress keeps flowing.

func TestStallContext_CancelsOnSilence(t *testing.T) {
	ctx, _, stop := stallContext(context.Background(), 30*time.Millisecond)
	defer stop()
	select {
	case <-ctx.Done(): // idle window elapsed with no keepAlive — correct
	case <-time.After(time.Second):
		t.Fatal("stall context not canceled after the idle window")
	}
}

func TestStallContext_KeepAliveDefersCancel(t *testing.T) {
	ctx, keepAlive, stop := stallContext(context.Background(), 120*time.Millisecond)
	defer stop()
	// Beat well inside the window repeatedly; the deadline must keep moving.
	for range 6 {
		time.Sleep(30 * time.Millisecond)
		keepAlive()
		if ctx.Err() != nil {
			t.Fatal("canceled despite keepAlive within the idle window")
		}
	}
}

func TestStallContext_StopCancels(t *testing.T) {
	ctx, _, stop := stallContext(context.Background(), time.Hour)
	stop()
	if ctx.Err() == nil {
		t.Error("stop must cancel the context")
	}
	stop() // idempotent — must not panic
}
