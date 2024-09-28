package message

import (
	"github.com/Tangerg/lynx/stream/codec"
)

func NewSimpleMessage(codecs ...codec.Codec) Message {
	var c = codec.GetDefaultCodec()
	if len(codecs) > 0 {
		c = codecs[0]
	}

	return &SimpleMessage{
		codeC:   c,
		payload: make([]byte, 0),
		headers: NewSimpleHeaders(),
	}
}

type SimpleMessage struct {
	codeC   codec.Codec
	payload []byte
	headers Headers
	err     error
}

func (s *SimpleMessage) Error() error {
	return s.err
}

func (s *SimpleMessage) Payload() []byte {
	return s.payload
}

func (s *SimpleMessage) Headers() Headers {
	return s.headers
}

func (s *SimpleMessage) SetPayload(v any) Message {
	if s.err != nil {
		return s
	}

	switch v.(type) {
	case []byte:
		s.payload = v.([]byte)
		return s
	}

	s.payload, s.err = s.codeC.Marshal(v)
	return s
}

func (s *SimpleMessage) SetHeaders(headers Headers) Message {
	if s.err != nil {
		return s
	}

	s.headers = headers
	return s
}

func (s *SimpleMessage) Unmarshal(v any) Message {
	if s.err != nil {
		return s
	}

	err := s.codeC.Unmarshal(s.payload, v)
	s.err = err
	return s
}
