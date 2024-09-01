package broker

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
)

type MockBroker struct {
	Empty bool
}

func (m *MockBroker) Produce(ctx context.Context, msgs ...*message.Msg) error {
	for _, msg := range msgs {
		fmt.Println("MockBroker Produce")
		fmt.Println(string(msg.Payload()))
	}
	return nil
}

func (m *MockBroker) Consume(ctx context.Context) (*message.Msg, message.ID, error) {
	if m.Empty {
		return nil, nil, nil
	}
	fmt.Println("MockBroker Consume")
	return message.New("Mock Msg"), 1, nil
}

func (m *MockBroker) Ack(ctx context.Context, id message.ID) error {
	fmt.Println("MockBroker Ack")
	fmt.Println(id)
	return nil
}

func (m *MockBroker) Close() error {
	return nil
}
