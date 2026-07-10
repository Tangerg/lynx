package a2a

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestConnectionsCloseRunsCleanupOnce(t *testing.T) {
	var calls atomic.Int32
	c := &Connections{close: func() error {
		calls.Add(1)
		return nil
	}}
	var callers sync.WaitGroup
	for range 16 {
		callers.Go(func() {
			if err := c.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		})
	}
	callers.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("cleanup calls = %d, want 1", got)
	}
}
