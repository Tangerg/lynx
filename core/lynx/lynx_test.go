package lynx

import (
	"github.com/Tangerg/lynx/core/broker"
	"github.com/Tangerg/lynx/core/job"
	"github.com/Tangerg/lynx/core/trigger"
	"github.com/Tangerg/lynx/core/worker"
	"testing"
)

func TestNew(t *testing.T) {
	bj := job.NewBatchJob(&job.BatchJobOptions{
		Trigger: trigger.NewCronTrigger(&trigger.CronTriggerOptions{
			Spec: "0/1 * * * * ?",
		}),
		Workers: []worker.BatchWorker{&worker.MockBatchWorker{}, &worker.MockBatchWorker{}, &worker.MockEmptyBatchWorker{}},
	})
	sj := job.NewStreamJob(&job.StreamJobOptions{
		Worker: &worker.MockStreamWorker{},
		Broker: &broker.MockBroker{},
		Config: &job.StreamJobConfig{
			MaxWork: 5,
		},
	})
	lynx := New(&Options{Jobs: []job.Job{bj, sj}})
	err := lynx.start()
	t.Log(err)
	lynx.wait()
	err = lynx.stop()
	t.Log(err)
}
