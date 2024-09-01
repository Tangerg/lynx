package trigger

import (
	"context"
	"github.com/Tangerg/lynx/core/worker"
	xsync "github.com/Tangerg/lynx/pkg/sync"
	"sync"
)

type CondTrigger struct {
	workers []worker.Worker
	cond    <-chan struct{}
	once    sync.Once
}

func NewCondTrigger(cond <-chan struct{}) *CondTrigger {
	return &CondTrigger{
		cond:    cond,
		workers: make([]worker.Worker, 0),
	}
}

func (c *CondTrigger) AddWorkers(ctx context.Context, workers ...worker.Worker) (int, error) {
	c.workers = append(c.workers, workers...)
	c.once.Do(func() {
		go c.listen(ctx)
	})
	return len(c.workers), nil
}

func (c *CondTrigger) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.cond:
			c.work()
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
