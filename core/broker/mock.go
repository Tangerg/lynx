package broker

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
)

type MockBroker struct {
	Empty bool
}

func (m *MockBroker) Produce(ctx context.Context, msgs map[string]message.Message) error {
	for _, msg := range msgs {
		fmt.Println("MockBroker Produce")
		fmt.Println(string(msg.Payload()))
	}
	return nil
}

func (m *MockBroker) Consume(ctx context.Context) (message.Message, error) {
	if m.Empty {
		return nil, nil
	}
	fmt.Println("MockBroker Consume")
	return message.NewSimpleMessage().SetPayload([]byte("MockBroker Consume")), nil
}

func (m *MockBroker) Ack(ctx context.Context, msg message.Message) error {
	fmt.Println("MockBroker Ack")
	fmt.Println(msg.Payload())
	return nil
}

func (m *MockBroker) Nack(ctx context.Context, msg message.Message) error {
	fmt.Println("MockBroker Nack")
	fmt.Println(msg.Payload())
	return nil
}

func (m *MockBroker) Close() error {
	return nil
}
