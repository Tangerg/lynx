package job

import (
	"context"
	"github.com/Tangerg/lynx/core/trigger"
	"github.com/Tangerg/lynx/core/worker"
	"sync"
	"sync/atomic"
)

type BatchJobOptions struct {
	Trigger trigger.Trigger
	Workers []worker.BatchWorker
}

type BatchJob struct {
	wg      sync.WaitGroup
	running atomic.Bool
	cancel  context.CancelFunc
	trigger trigger.Trigger
	workers []worker.BatchWorker
}

func NewBatchJob(opt *BatchJobOptions) Job {
	return &BatchJob{
		trigger: opt.Trigger,
		workers: opt.Workers,
	}
}

func (b *BatchJob) Start(ctx context.Context) error {
	if b.running.Load() {
		return nil
	}
	b.running.Store(true)
	nctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	return b.run(nctx)
}

func (b *BatchJob) Stop() error {
	if !b.running.Load() {
		return nil
	}
	b.running.Store(false)
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	return nil
}

func (b *BatchJob) run(ctx context.Context) error {
	workers := make([]worker.Worker, 0, len(b.workers))
	for _, w := range b.workers {
		w.Context(ctx)
		workers = append(workers, w)
		go func(bw worker.BatchWorker) {
			b.listenWorkerDone(bw)
		}(w)
	}
	count, err := b.trigger.AddWorkers(ctx, workers...)
	if err != nil {
		return err
	}
	b.wg.Add(count)

	return err
}

func (b *BatchJob) listenWorkerDone(w worker.BatchWorker) {
	defer b.wg.Done()
	for {
		select {
		case <-w.Done():
			return
		}
	}
}
