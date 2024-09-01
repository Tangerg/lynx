package job

import (
	"context"
	"github.com/Tangerg/lynx/core/broker"
	"github.com/Tangerg/lynx/core/worker"
	"testing"
	"time"
)

func TestNewStreamJob(t *testing.T) {
	sj := NewStreamJob(&StreamJobOptions{
		Worker: &worker.MockStreamWorker{},
		Broker: &broker.MockBroker{},
		Config: &StreamJobConfig{
			MaxWork: 5,
		},
	})
	err := sj.Start(context.Background())
	t.Log(err)
	time.Sleep(10 * time.Second)
	err = sj.Stop()
	t.Log(err)
	time.Sleep(2 * time.Second)
}

func TestNewStreamJob2(t *testing.T) {
	sj := NewStreamJob(&StreamJobOptions{
		Worker: &worker.MockStreamWorker{},
		Broker: &broker.MockBroker{Empty: true},
		Config: &StreamJobConfig{
			MaxWork: 5,
		},
	})
	err := sj.Start(context.Background())
	t.Log(err)
	time.Sleep(10 * time.Second)
	err = sj.Stop()
	t.Log(err)
	time.Sleep(2 * time.Second)
}
