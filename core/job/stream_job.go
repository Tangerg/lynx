package job

import (
	"context"
	"github.com/Tangerg/lynx/core/broker"
	"github.com/Tangerg/lynx/core/worker"
	xsync "github.com/Tangerg/lynx/pkg/sync"
	"log/slog"
	"sync"
	"sync/atomic"
)

type StreamJobConfig struct {
	MaxWork int `yaml:"MaxWorker"`
}

type StreamJobOptions struct {
	Config *StreamJobConfig
	Worker worker.StreamWorker
	Broker broker.Broker
}

type StreamJob struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool
	limiter *xsync.Limiter
	worker  worker.StreamWorker
	broker  broker.Broker
}

func NewStreamJob(opt *StreamJobOptions) Job {
	return &StreamJob{
		limiter: xsync.NewLimiter(opt.Config.MaxWork),
		worker:  opt.Worker,
		broker:  opt.Broker,
	}
}

func (s *StreamJob) Start(ctx context.Context) error {
	if s.running.Load() {
		return nil
	}
	s.running.Store(true)
	nctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	xsync.Go(func() {
		s.run(nctx)
	})
	return nil
}

func (s *StreamJob) Stop() error {
	if !s.running.Load() {
		return nil
	}
	s.running.Store(false)
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *StreamJob) run(ctx context.Context) {
	for {
		s.limiter.Acquire()
		if !s.running.Load() {
			return
		}
		s.wg.Add(1)
		xsync.Go(func() {
			err := s.work(ctx)
			if err != nil {
				slog.Error("job err", slog.String("err", err.Error()))
			}
		})
	}
}

func (s *StreamJob) work(ctx context.Context) error {
	defer s.wg.Done()
	defer s.limiter.Release()

	msg, err := s.broker.Consume(ctx)
	if err != nil {
		return err
	}
	if msg == nil {
		s.worker.Sleep()
		return nil
	}
	msgs, err := s.worker.Work(ctx, msg)
	if err != nil {
		return s.broker.Nack(ctx, msg)
	}
	if len(msgs) == 0 {
		return s.broker.Ack(ctx, msg)
	}
	err = s.broker.Produce(ctx, msgs)
	if err != nil {
		return err
	}
	return s.broker.Ack(ctx, msg)
}
