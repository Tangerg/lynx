package slices

import (
	"testing"
)

func TestSplit(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, strings := range Split(s, 3) {
		t.Log(strings)
	}
}
