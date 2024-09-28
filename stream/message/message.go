package message

type Message interface {
	Payload() []byte
	Headers() Headers
	SetPayload(v any) Message
	SetHeaders(h Headers) Message
	Unmarshal(v any) Message
	Error() error
}
