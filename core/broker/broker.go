package broker

import (
	"context"
	"github.com/Tangerg/lynx/core/msg"
	"io"
)

type Producer interface {
	Produce(ctx context.Context, msgs ...*msg.Msg) error
}
type Consumer interface {
	Consume(ctx context.Context) (*msg.Msg, msg.ID, error)
	Ack(ctx context.Context, msg msg.ID) error
}

type Broker interface {
	Producer
	Consumer
	io.Closer
}
