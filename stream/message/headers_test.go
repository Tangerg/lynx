package message

import (
	"testing"
)

func TestSimpleHeaders_Set(t *testing.T) {
	headers := NewSimpleHeaders()
	headers.Set("test", "test").
		Set("test1", 123)
}

func TestSimpleHeaders_Get(t *testing.T) {
	headers := NewSimpleHeaders()
	headers.Set("test", "test").
		Set("test1", 123)
	t.Log(headers.Get("test"))
	t.Log(headers.Get("test1"))
}
