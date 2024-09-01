package trigger

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/core/worker"
	"github.com/robfig/cron/v3"
	"sync"
)

type CronTriggerOptions struct {
	Spec string `yaml:"Spec"`
}

type CronTrigger struct {
	spec string
	cron *cron.Cron
	once sync.Once
}

func NewCronTriggerWithCron(cron *cron.Cron, opt *CronTriggerOptions) Trigger {
	return &CronTrigger{
		spec: opt.Spec,
		cron: cron,
	}
}

func NewCronTrigger(opt *CronTriggerOptions) Trigger {
	return &CronTrigger{
		spec: opt.Spec,
		cron: cron.New(cron.WithSeconds()),
	}
}

func (c *CronTrigger) AddWorkers(ctx context.Context, workers ...worker.Worker) (int, error) {
	errs := make([]error, 0, len(workers))
	var count = 0
	for _, w := range workers {
		_, err := c.cron.AddFunc(c.spec, w.Work)
		if err != nil {
			errs = append(errs, err)
		} else {
			count++
		}
	}
	c.once.Do(func() {
		c.cron.Start()
		go c.listen(ctx)
	})
	return count, errors.Join(errs...)
}

func (c *CronTrigger) listen(ctx context.Context) {
	defer c.cron.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
