package message

import "testing"

func TestMarshal(t *testing.T) {
	v, err := Marshal(123)
	t.Log(v)
	t.Log(err)
}
func TestUnmarshal(t *testing.T) {
	var v int
	err := Unmarshal([]byte{49, 50, 51}, &v)
	t.Log(v)
	t.Log(err)
}
