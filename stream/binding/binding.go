package binding

import (
	"context"

	"github.com/Tangerg/lynx/stream/message"
)

type Direction int

const (
	_ Direction = iota
	Send
	Receive
	SendAndReceive
)

type Binding interface {
	Send(ctx context.Context, message message.Message) error
	Receive(ctx context.Context) (message.Message, error)
	Ack(ctx context.Context, message message.Message) error
	Nack(ctx context.Context, message message.Message) error
}
