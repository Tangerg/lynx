package message

import "encoding/json"

var defaultCodec Codec

type Codec interface {
	Marshal(data any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

func init() {
	SetDefaultCodec(NewJsonCodec())
}

func SetDefaultCodec(c Codec) {
	defaultCodec = c
}

func NewJsonCodec() Codec {
	return &jsonCodec{}
}

type jsonCodec struct{}

func (c *jsonCodec) Marshal(data any) ([]byte, error) {
	return json.Marshal(data)
}
func (c *jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func Marshal(data any) ([]byte, error) {
	return defaultCodec.Marshal(data)
}
func Unmarshal(data []byte, v any) error {
	return defaultCodec.Unmarshal(data, v)
}
