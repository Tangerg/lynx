package slices

import (
	"testing"
)

func TestEnsureIndex(t *testing.T) {
	s := []int{1, 2, 3}
	ints := EnsureIndex(s, 4)
	t.Log(cap(ints))
	t.Log(len(ints))
}

func TestChunk(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, strings := range Chunk(s, 3) {
		t.Log(strings)
	}
}

func TestAt(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for i, _ := range s {
		e, ok := At(s, i)
		t.Log(ok, e)
		t.Log("---")
		e, ok = At(s, i+1)
		t.Log(ok, e)
	}
	t.Log(At(s, -1))
}

func TestAtOr(t *testing.T) {
	s := []string{"a", "b", "c"}
	e := AtOr(s, 2, "d")
	t.Log(e)
	e = AtOr(s, 3, "d")
	t.Log(e)
}
