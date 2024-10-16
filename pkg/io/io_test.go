package io

import (
	"strings"
	"testing"
)

func TestReadBuffer(t *testing.T) {
	buffer, err := ReadAll(strings.NewReader("hello world"), 128)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(buffer)
	t.Log(string(buffer))
}
