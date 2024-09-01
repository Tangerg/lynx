package broker

import (
	"context"
	"github.com/Tangerg/lynx/core/message"
	"io"
)

type Producer interface {
	Produce(ctx context.Context, msgs ...*message.Msg) error
}
type Consumer interface {
	Consume(ctx context.Context) (*message.Msg, message.ID, error)
	Ack(ctx context.Context, id message.ID) error
}

type Broker interface {
	Producer
	Consumer
	io.Closer
}
