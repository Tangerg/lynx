package random

import (
	"testing"
)

func TestInt(t *testing.T) {
	i := IntRange(-100, 50)
	t.Log(i)

	i = IntRange(-2, 2)
	t.Log(i)

	i = IntRange(-3, 2)
	t.Log(i)

	i = IntRange(-4, 2)
	t.Log(i)

	i = IntRange(-4, 2)
	t.Log(i)

	i = IntRange(2, 100)
	t.Log(i)
}

func TestRand(t *testing.T) {
	for i := 0; i < 100000000; i++ {
		j := IntRange(-100, 100)
		if j < -100 || j > 100 {
			panic("j out of range")
		}
	}
}

func BenchmarkRand(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		IntRange(-100, 100)
	}
}
