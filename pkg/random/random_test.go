package random

import (
	"strings"
	"sync"
	"testing"
)

func TestIntRange_InRange(t *testing.T) {
	tests := []struct{ lo, hi int }{
		{0, 10},
		{0, 1000000},
		{10, 20},
		{-10, 10},
		{-1000, -900},
		{-100, 0},
	}
	const iterations = 200
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			for range iterations {
				got := IntRange(tt.lo, tt.hi)
				if got < tt.lo || got >= tt.hi {
					t.Fatalf("IntRange(%d, %d) = %d, out of [%d, %d)", tt.lo, tt.hi, got, tt.lo, tt.hi)
				}
			}
		})
	}
}

func TestIntRange_SingleValue(t *testing.T) {
	tests := []struct {
		lo, hi, want int
	}{
		{0, 1, 0},
		{5, 6, 5},
		{-1, 0, -1},
		{999, 1000, 999},
	}
	for _, tt := range tests {
		for range 10 {
			if got := IntRange(tt.lo, tt.hi); got != tt.want {
				t.Errorf("IntRange(%d, %d) = %d, want %d", tt.lo, tt.hi, got, tt.want)
			}
		}
	}
}

func TestIntRange_PanicsOnInvalidArgs(t *testing.T) {
	tests := []struct{ lo, hi int }{
		{10, 5},
		{0, 0},    // lo == hi
		{10, -10}, // lo > hi
		{1, 0},    // off-by-one
		{-5, -10}, // both negative reversed
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("IntRange(%d, %d) did not panic", tt.lo, tt.hi)
				}
				msg, ok := r.(string)
				if !ok {
					t.Fatalf("panic value type = %T, want string", r)
				}
				if !strings.Contains(msg, "lo must be less than hi") {
					t.Errorf("panic msg = %q", msg)
				}
			}()
			_ = IntRange(tt.lo, tt.hi)
		})
	}
}

func TestIntRange_CoversFullRange(t *testing.T) {
	const lo, hi = 0, 5
	const iterations = 1000
	seen := make(map[int]int, hi-lo)
	for range iterations {
		seen[IntRange(lo, hi)]++
	}
	for v := range hi - lo {
		if seen[lo+v] == 0 {
			t.Errorf("value %d never produced in %d iterations", lo+v, iterations)
		}
	}
	for v := range seen {
		if v < lo || v >= hi {
			t.Errorf("value %d outside [%d, %d)", v, lo, hi)
		}
	}
}

func TestIntRange_Concurrent(t *testing.T) {
	const goroutines, iterations = 10, 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				if v := IntRange(0, 100); v < 0 || v >= 100 {
					t.Errorf("got %d, out of [0, 100)", v)
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkIntRange(b *testing.B) {
	b.Run("small", func(b *testing.B) {
		for b.Loop() {
			_ = IntRange(0, 10)
		}
	})
	b.Run("large", func(b *testing.B) {
		for b.Loop() {
			_ = IntRange(0, 1_000_000)
		}
	})
}
