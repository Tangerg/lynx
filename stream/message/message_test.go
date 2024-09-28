package message

import (
	"testing"
)

func TestNewSimpleMessage(t *testing.T) {
	message := NewSimpleMessage()
	message.SetPayload("payload")
	message.Headers().Set("test", "test")
	t.Log(message.Error())
}

func TestNewSimpleMessage1(t *testing.T) {
	message := NewSimpleMessage()
	message.SetPayload("payload")
	message.Headers().Set("test", "test")
	t.Log(message.Error())
	var s string
	message.Unmarshal(&s)
	t.Log(s)
	t.Log(message.Error())
}
