package worker

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
	"sync"
	"time"
)

type MockEmptyWorker struct{}

func (m *MockEmptyWorker) Work() {}

type MockWorker struct {
	MockEmptyWorker
}

func (m *MockWorker) Work() {
	fmt.Println("MockWorker Work")
}

type MockEmptyBatchWorker struct {
	ctx  context.Context
	once sync.Once
}

func (m *MockEmptyBatchWorker) Context(ctx context.Context) {
	m.ctx = ctx
}

func (m *MockEmptyBatchWorker) Done() <-chan struct{} {
	return m.ctx.Done()
}

func (m *MockEmptyBatchWorker) Work() {}

type MockBatchWorker struct {
	MockEmptyBatchWorker
}

func (m *MockBatchWorker) Work() {
	fmt.Println("MockBatchWorker Work")
}

type MockStreamWorker struct{}

func (m *MockStreamWorker) Sleep() {
	fmt.Println("MockStreamWorker Sleep")
	time.Sleep(1 * time.Second)
}
func (m *MockStreamWorker) Work(ctx context.Context, msg message.Message) (map[string]message.Message, error) {
	fmt.Println("MockStreamWorker Work")
	fmt.Println(string(msg.Payload()))
	return make(map[string]message.Message), nil
}
