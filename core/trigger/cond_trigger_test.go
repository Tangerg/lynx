package trigger

import (
	"context"
	"github.com/Tangerg/lynx/core/worker"
	"testing"
	"time"
)

func TestNewCondTrigger(t *testing.T) {
	c := make(chan struct{}, 1)
	ct := NewCondTrigger(c)
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{})
	c <- struct{}{}
	c <- struct{}{}
	c <- struct{}{}
	time.Sleep(2 * time.Second)
	cancel()
	c <- struct{}{}
}
func TestNewCondTrigger2(t *testing.T) {
	c := make(chan struct{}, 1)
	ct := NewCondTrigger(c)
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{}, &worker.MockWorker{})
	c <- struct{}{}
	c <- struct{}{}
	c <- struct{}{}
	time.Sleep(2 * time.Second)
	cancel()
	c <- struct{}{}
}

func BenchmarkNewCondTrigger(b *testing.B) {
	c := make(chan struct{}, 1)
	ct := NewCondTrigger(c)
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockEmptyWorker{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c <- struct{}{}
	}
	b.StopTimer()
	time.Sleep(2 * time.Second)
	cancel()
	c <- struct{}{}
}
