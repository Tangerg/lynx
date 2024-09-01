package message

type ID any
type Msg struct {
	payload []byte
}

func New(payload any) *Msg {
	switch payload.(type) {
	case []byte:
		return &Msg{payload: payload.([]byte)}
	}
	v, _ := Marshal(payload)
	return &Msg{payload: v}
}

func (m *Msg) Payload() []byte {
	return m.payload
}

func (m *Msg) Unmarshal(v any) error {
	return Unmarshal(m.payload, v)
}
