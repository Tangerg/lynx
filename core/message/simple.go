package message

type SimpleMessage struct {
	payload []byte
	headers Headers
}

func NewSimpleMessage() Message {
	return &SimpleMessage{}
}

func (s *SimpleMessage) Payload() []byte {
	return s.payload
}

func (s *SimpleMessage) SetPayload(bytes []byte) Message {
	s.payload = bytes
	return s
}

func (s *SimpleMessage) Headers() Headers {
	return s.headers
}

func (s *SimpleMessage) SetHeaders(headers Headers) Message {
	s.headers = headers
	return s
}

func (s *SimpleMessage) Unmarshal(v any) error {
	return Unmarshal(s.payload, v)
}
