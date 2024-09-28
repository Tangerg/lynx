package binding

import (
	"context"

	"github.com/Tangerg/lynx/stream/message"
)

// Direction represents the direction of message flow in a binding.
type Direction int

const (
	_              Direction = iota // Skip the zero value for Direction
	Send                            // Send indicates that the binding is used for sending messages.
	Receive                         // Receive indicates that the binding is used for receiving messages.
	SendAndReceive                  // SendAndReceive indicates that the binding can both send and receive messages.
)

// Binding is an interface that defines methods for sending, receiving, acknowledging,
// and negatively acknowledging messages in a message stream.
type Binding interface {
	// Send sends a message to the stream.
	// It takes a context for managing request-scoped values and cancellation signals.
	Send(ctx context.Context, message message.Message) error

	// Receive receives a message from the stream.
	// It returns the received message and any error encountered.
	Receive(ctx context.Context) (message.Message, error)

	// Ack acknowledges the successful processing of a message.
	// It takes a context and the message to be acknowledged.
	Ack(ctx context.Context, message message.Message) error

	// Nack negatively acknowledges a message, indicating that it was not processed successfully.
	// It takes a context and the message to be negatively acknowledged.
	Nack(ctx context.Context, message message.Message) error
}
