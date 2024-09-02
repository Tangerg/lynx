package trigger

import (
	"context"
	"github.com/Tangerg/lynx/core/worker"
	xsync "github.com/Tangerg/lynx/pkg/sync"
	"sync"
	"sync/atomic"
)

type CondTrigger struct {
	running atomic.Bool
	workers []worker.Worker
	cond    *sync.Cond
	once    sync.Once
}

func NewCondTrigger(cond *sync.Cond) *CondTrigger {
	return &CondTrigger{
		cond:    cond,
		workers: make([]worker.Worker, 0),
	}
}

func (c *CondTrigger) AddWorkers(ctx context.Context, workers ...worker.Worker) (int, error) {
	c.workers = append(c.workers, workers...)
	c.once.Do(func() {
		c.running.Store(true)
		go c.listenCtx(ctx)
		go c.listenCond()
	})
	return len(c.workers), nil
}

func (c *CondTrigger) listenCond() {
	for {
		c.cond.L.Lock()
		c.cond.Wait()
		c.cond.L.Unlock()
		if !c.running.Load() {
			return
		}
		c.work()
	}
}

func (c *CondTrigger) listenCtx(ctx context.Context) {
	defer c.running.Store(false)
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}

func (c *CondTrigger) work() {
	for _, w := range c.workers {
		xsync.Go(func() {
			w.Work()
		})
	}
}
