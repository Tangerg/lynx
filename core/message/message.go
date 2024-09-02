package message

type Message interface {
	Payload() []byte
	SetPayload(payload []byte) Message
	Headers() Headers
	SetHeaders(headers Headers) Message
	Unmarshal(v any) error
}
