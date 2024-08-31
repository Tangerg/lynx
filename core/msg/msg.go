package msg

import "encoding/json"

type ID any
type Msg struct {
	payload []byte
}

func New(payload any) *Msg {
	switch payload.(type) {
	case []byte:
		return &Msg{payload: payload.([]byte)}
	}
	v, _ := json.Marshal(payload)
	return &Msg{payload: v}
}

func (m *Msg) Payload() []byte {
	return m.payload
}
func (m *Msg) Unmarshal(v any) error {
	return json.Unmarshal(m.payload, v)
}
