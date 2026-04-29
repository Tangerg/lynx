package sync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	l := NewLimiter(3)
	if l == nil {
		t.Fatal("nil limiter")
	}
	if cap(l.slots) != 3 {
		t.Errorf("cap = %d, want 3", cap(l.slots))
	}
}

func TestNewLimiter_PanicsOnNonPositive(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("NewLimiter(%d) did not panic", n)
				}
			}()
			_ = NewLimiter(n)
		})
	}
}

func TestLimiter_AcquireRelease(t *testing.T) {
	l := NewLimiter(2)
	l.Acquire()
	l.Acquire()
	if len(l.slots) != 2 {
		t.Errorf("active = %d, want 2", len(l.slots))
	}
	l.Release()
	l.Release()
	if len(l.slots) != 0 {
		t.Errorf("active = %d, want 0", len(l.slots))
	}
}

func TestLimiter_BoundsConcurrency(t *testing.T) {
	const max = 3
	l := NewLimiter(max)
	var active, peak atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Acquire()
			defer l.Release()
			n := active.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
		}()
	}
	wg.Wait()
	if peak.Load() > max {
		t.Errorf("peak = %d, want <= %d", peak.Load(), max)
	}
}

func TestLimiter_AcquireBlocksWhenFull(t *testing.T) {
	l := NewLimiter(1)
	l.Acquire()
	done := make(chan struct{})
	go func() {
		l.Acquire()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Acquire did not block")
	case <-time.After(50 * time.Millisecond):
	}
	l.Release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Acquire did not unblock after Release")
	}
	l.Release()
}

func BenchmarkLimiter_AcquireRelease(b *testing.B) {
	l := NewLimiter(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Acquire()
			l.Release()
		}
	})
}
