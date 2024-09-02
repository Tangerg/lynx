package trigger

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/core/worker"
	"sync"
	"testing"
	"time"
)

func TestNewCondTrigger(t *testing.T) {
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)
	ct := NewCondTrigger(cond)
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{})
	cond.Broadcast()
	time.Sleep(2 * time.Second)
	cond.Broadcast()
	time.Sleep(2 * time.Second)
	cond.Signal()
	time.Sleep(2 * time.Second)
	cancel()
	cond.Broadcast()
	time.Sleep(2 * time.Second)
}
func TestNewCondTrigger2(t *testing.T) {
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)
	ct := NewCondTrigger(cond)
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{}, &worker.MockWorker{}, &worker.MockWorker{})
	fmt.Println("1")
	cond.Broadcast()
	time.Sleep(2 * time.Second)
	fmt.Println("2")
	cond.Broadcast()
	time.Sleep(2 * time.Second)
	fmt.Println("Signal")
	cond.Signal()
	time.Sleep(2 * time.Second)
	fmt.Println("cancel")
	cancel()
	time.Sleep(2 * time.Second)
	fmt.Println("3")
	cond.Broadcast()
	time.Sleep(2 * time.Second)
}
