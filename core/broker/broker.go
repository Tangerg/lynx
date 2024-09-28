package broker

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/core/message"
)

// messageID The identifier used to identify each broker message
const messageID = "messageID"

type Producer interface {
	// Produce Publish sends messages to the specified Topics
	Produce(ctx context.Context, msgs map[string]message.Message) error
}

type Consumer interface {
	// Consume Subscribe registers a consumer to a topic
	// The Topic should be specified during the initialization of the consumer instance
	Consume(ctx context.Context) (message.Message, error)
	// Ack acknowledges the successful processing of a message
	Ack(ctx context.Context, msg message.Message) error
	// Nack negatively acknowledges a message, indicating processing failure
	Nack(ctx context.Context, msg message.Message) error
}

type Broker interface {
	Producer
	Consumer
	io.Closer
}
