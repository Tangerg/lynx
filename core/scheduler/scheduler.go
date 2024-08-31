package scheduler

import (
	"context"
	"github.com/Tangerg/lynx/core/broker"
	"github.com/Tangerg/lynx/core/worker"
	xsync "github.com/Tangerg/lynx/pkg/sync"
	"log/slog"
	"sync"
	"sync/atomic"
)

type Config struct {
	MaxWorker int `yaml:"MaxWorker"`
}
type Options struct {
	Config *Config
	Worker worker.Worker
	Broker broker.Broker
}

type Scheduler struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	stopped atomic.Bool
	limiter *xsync.Limiter
	worker  worker.Worker
	broker  broker.Broker
}

func New(opt *Options) *Scheduler {
	return &Scheduler{
		limiter: xsync.NewLimiter(opt.Config.MaxWorker),
		worker:  opt.Worker,
		broker:  opt.Broker,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	nctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	xsync.Go(func() {
		s.run(nctx)
	})
}

func (s *Scheduler) Stop() {
	s.stopped.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) run(ctx context.Context) {
	for {
		s.limiter.Acquire()
		if s.stopped.Load() {
			return
		}
		s.wg.Add(1)
		xsync.Go(func() {
			defer s.wg.Done()
			defer s.limiter.Release()
			err := s.work(ctx)
			if err != nil {
				slog.Error("scheduler err", slog.String("err", err.Error()))
			}
		})
	}
}

func (s *Scheduler) work(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	msg, msgId, err := s.broker.Consume(ctx)
	if err != nil {
		return err
	}
	if msg == nil {
		s.worker.Sleep()
		return nil
	}
	msgs, err := s.worker.Work(ctx, msg)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return s.broker.Ack(ctx, msgId)
	}
	err = s.broker.Produce(ctx, msgs...)
	if err != nil {
		return err
	}
	return s.broker.Ack(ctx, msgId)
}
