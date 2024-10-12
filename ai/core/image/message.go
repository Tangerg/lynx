package image

func NewMessage(text string, weight float64) *Message {
	return &Message{
		text:   text,
		weight: weight,
	}
}

type Message struct {
	text   string
	weight float64
}

func (m *Message) Text() string {
	return m.text
}

func (m *Message) Weight() float64 {
	return m.weight
}
