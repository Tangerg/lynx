package job

import (
	"context"
	"github.com/Tangerg/lynx/core/trigger"
	"github.com/Tangerg/lynx/core/worker"
	"testing"
	"time"
)

func TestNewBatchJob(t *testing.T) {
	bj := NewBatchJob(&BatchJobOptions{
		Trigger: trigger.NewCronTrigger(&trigger.CronTriggerOptions{
			Spec: "0/1 * * * * ?",
		}),
		Workers: []worker.BatchWorker{&worker.MockBatchWorker{}, &worker.MockBatchWorker{}, &worker.MockEmptyBatchWorker{}},
	})
	err := bj.Start(context.Background())
	t.Log(err)
	time.Sleep(5 * time.Second)
	err = bj.Stop()
	t.Log(err)
	time.Sleep(5 * time.Second)
}
