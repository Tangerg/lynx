package random

import (
	"testing"
)

func TestInt(t *testing.T) {
	i := Int(-100, 50)
	t.Log(i)

	i = Int(-2, 2)
	t.Log(i)

	i = Int(-3, 2)
	t.Log(i)

	i = Int(-4, 2)
	t.Log(i)

	i = Int(-4, 2)
	t.Log(i)

	i = Int(2, 100)
	t.Log(i)
}

func TestRand(t *testing.T) {
	for i := 0; i < 100000000; i++ {
		j := Int(-100, 100)
		if j < -100 || j > 100 {
			panic("j out of range")
		}
	}
}

func BenchmarkRand(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Int(-100, 100)
	}
}
