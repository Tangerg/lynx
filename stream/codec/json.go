package codec

import (
	"encoding/json"
)

func NewJsonCodec() Codec {
	return &Json{}
}

type Json struct{}

func (j *Json) Marshal(data any) ([]byte, error) {
	return json.Marshal(data)
}
func (j *Json) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
