package message

import "testing"

func TestNew(t *testing.T) {
	msg := New(123)
	t.Log(msg)
	t.Log(msg.Payload())
	msg = New("abc")
	t.Log(msg)
	t.Log(msg.Payload())
	msg = New(struct {
		A int    `yaml:"A"`
		B string `yaml:"B"`
	}{
		A: 1,
		B: "B",
	})
	t.Log(msg)
	t.Log(msg.Payload())
	msg = New([]byte{1, 2, 3})
	t.Log(msg)
	t.Log(msg.Payload())
}

func TestMsg_Unmarshal(t *testing.T) {
	msg := New(struct {
		A int    `yaml:"A"`
		B string `yaml:"B"`
	}{
		A: 1,
		B: "B",
	})
	m := make(map[string]any)
	err := msg.Unmarshal(&m)
	t.Log(err)
	t.Log(m)
}
