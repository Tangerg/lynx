package message

// Message is an interface that defines methods for handling message data.
// It provides methods to access and modify the payload and headers, as well as error handling.
type Message interface {
	// Payload returns the message payload as a byte slice.
	Payload() []byte

	// Headers returns the headers associated with the message.
	Headers() Headers

	// SetPayload sets the message payload to the specified value.
	// It returns the Message interface to allow for method chaining.
	SetPayload(v any) Message

	// SetHeaders sets the headers for the message.
	// It returns the Message interface to allow for method chaining.
	SetHeaders(h Headers) Message

	// Unmarshal decodes the message payload into the specified value.
	// It returns the Message interface to allow for method chaining.
	Unmarshal(v any) Message

	// Error returns an error if there is an issue with the message.
	Error() error
}
