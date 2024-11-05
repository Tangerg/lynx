package message

func NewMessage(text string, weight float64) *ImageMessage {
	return &ImageMessage{
		text:   text,
		weight: weight,
	}
}

type ImageMessage struct {
	text   string
	weight float64
}

func (m *ImageMessage) Text() string {
	return m.text
}

func (m *ImageMessage) Weight() float64 {
	return m.weight
}
