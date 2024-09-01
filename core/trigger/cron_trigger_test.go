package trigger

import (
	"context"
	"github.com/Tangerg/lynx/core/worker"
	"testing"
	"time"
)

func TestNewCronTrigger(t *testing.T) {
	ct := NewCronTrigger(&CronTriggerOptions{
		Spec: "0/1 * * * * ?",
	})
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{})
	time.Sleep(5 * time.Second)
	cancel()
	time.Sleep(5 * time.Second)
}

func TestNewCronTrigger2(t *testing.T) {
	ct := NewCronTrigger(&CronTriggerOptions{
		Spec: "0/1 * * * * ?",
	})
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{}, &worker.MockWorker{})
	_, _ = ct.AddWorkers(ctx, &worker.MockWorker{}, &worker.MockWorker{})
	time.Sleep(5 * time.Second)
	cancel()
	time.Sleep(5 * time.Second)
}
