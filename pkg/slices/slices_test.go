package slices

import (
	"reflect"
	"testing"
)

func TestEnsureIndex(t *testing.T) {
	t.Run("index within length returns same data", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5}
		got := EnsureIndex(s, 2)
		if !reflect.DeepEqual(got, s) || len(got) != 5 {
			t.Errorf("got %v len=%d, want %v", got, len(got), s)
		}
	})

	t.Run("extends within capacity", func(t *testing.T) {
		s := make([]int, 3, 10)
		s[0], s[1], s[2] = 1, 2, 3
		got := EnsureIndex(s, 5)
		if len(got) != 6 || cap(got) != 10 {
			t.Errorf("len=%d cap=%d, want len=6 cap=10", len(got), cap(got))
		}
		for i, want := range []int{1, 2, 3, 0, 0, 0} {
			if got[i] != want {
				t.Errorf("got[%d] = %d, want %d", i, got[i], want)
			}
		}
	})

	t.Run("allocates beyond capacity", func(t *testing.T) {
		s := []int{1, 2, 3}
		got := EnsureIndex(s, 5)
		if len(got) != 6 {
			t.Errorf("len = %d, want 6", len(got))
		}
		for i, want := range []int{1, 2, 3, 0, 0, 0} {
			if got[i] != want {
				t.Errorf("got[%d] = %d, want %d", i, got[i], want)
			}
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		var s []int
		got := EnsureIndex(s, 3)
		if len(got) != 4 {
			t.Errorf("len = %d, want 4", len(got))
		}
	})

	t.Run("zero index extends to length 1", func(t *testing.T) {
		var s []int
		got := EnsureIndex(s, 0)
		if len(got) != 1 || got[0] != 0 {
			t.Errorf("got %v len=%d, want [0]", got, len(got))
		}
	})

	t.Run("panics on negative index", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		_ = EnsureIndex([]int{1}, -1)
	})

	t.Run("custom slice type", func(t *testing.T) {
		type Vec []int
		v := Vec{1, 2, 3}
		got := EnsureIndex(v, 5)
		if _, ok := any(got).(Vec); !ok {
			t.Errorf("type lost, got %T", got)
		}
		if len(got) != 6 {
			t.Errorf("len = %d, want 6", len(got))
		}
	})
}

func TestChunk(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		size int
		want [][]int
	}{
		{"even split", []int{1, 2, 3, 4, 5, 6}, 2, [][]int{{1, 2}, {3, 4}, {5, 6}}},
		{"uneven", []int{1, 2, 3, 4, 5}, 3, [][]int{{1, 2, 3}, {4, 5}}},
		{"size > len", []int{1, 2, 3}, 10, [][]int{{1, 2, 3}}},
		{"size 1", []int{1, 2, 3}, 1, [][]int{{1}, {2}, {3}}},
		{"empty", []int{}, 3, [][]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Chunk(tt.in, tt.size)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i, c := range got {
				if !reflect.DeepEqual([]int(c), tt.want[i]) {
					t.Errorf("chunk[%d] = %v, want %v", i, c, tt.want[i])
				}
			}
		})
	}

	t.Run("panics on non-positive size", func(t *testing.T) {
		for _, sz := range []int{0, -1, -100} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("Chunk(_, %d) did not panic", sz)
					}
				}()
				_ = Chunk([]int{1, 2}, sz)
			}()
		}
	})

	t.Run("chunk capacity is bounded", func(t *testing.T) {
		s := []int{1, 2, 3, 4}
		chunks := Chunk(s, 2)
		// Appending to chunk[0] must not corrupt chunk[1].
		chunks[0] = append(chunks[0], 99)
		if chunks[1][0] != 3 {
			t.Errorf("chunk[1][0] = %d, want 3 (chunk[0] append leaked)", chunks[1][0])
		}
	})
}

func TestAt(t *testing.T) {
	tests := []struct {
		name   string
		s      []int
		i      int
		want   int
		wantOk bool
	}{
		{"positive in-range", []int{10, 20, 30}, 1, 20, true},
		{"first", []int{10, 20, 30}, 0, 10, true},
		{"negative -1", []int{10, 20, 30}, -1, 30, true},
		{"negative -2", []int{10, 20, 30}, -2, 20, true},
		{"out of range high", []int{10, 20, 30}, 10, 0, false},
		{"out of range low", []int{10, 20, 30}, -10, 0, false},
		{"empty slice", []int{}, 0, 0, false},
		{"empty slice negative", []int{}, -1, 0, false},
		{"nil slice", nil, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := At(tt.s, tt.i)
			if got != tt.want || ok != tt.wantOk {
				t.Errorf("At(%v, %d) = (%d, %v), want (%d, %v)", tt.s, tt.i, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

func TestAtOr(t *testing.T) {
	tests := []struct {
		s    []int
		i    int
		or   int
		want int
	}{
		{[]int{10, 20, 30}, 1, -1, 20},
		{[]int{10, 20, 30}, -1, -1, 30},
		{[]int{10, 20, 30}, 99, -1, -1},
		{[]int{}, 0, 42, 42},
		{nil, 0, 42, 42},
	}
	for _, tt := range tests {
		if got := AtOr(tt.s, tt.i, tt.or); got != tt.want {
			t.Errorf("AtOr(%v, %d, %d) = %d, want %d", tt.s, tt.i, tt.or, got, tt.want)
		}
	}
}

func TestFirstLast(t *testing.T) {
	t.Run("non-empty", func(t *testing.T) {
		s := []int{10, 20, 30}
		if v, ok := First(s); v != 10 || !ok {
			t.Errorf("First = (%d, %v), want (10, true)", v, ok)
		}
		if v, ok := Last(s); v != 30 || !ok {
			t.Errorf("Last = (%d, %v), want (30, true)", v, ok)
		}
	})
	t.Run("empty", func(t *testing.T) {
		var s []int
		if _, ok := First(s); ok {
			t.Error("First on empty returned ok=true")
		}
		if _, ok := Last(s); ok {
			t.Error("Last on empty returned ok=true")
		}
	})
	t.Run("FirstOr/LastOr fallback", func(t *testing.T) {
		s := []int{10, 20, 30}
		if got := FirstOr(s, -1); got != 10 {
			t.Errorf("FirstOr = %d, want 10", got)
		}
		if got := LastOr(s, -1); got != 30 {
			t.Errorf("LastOr = %d, want 30", got)
		}
		var empty []int
		if got := FirstOr(empty, -1); got != -1 {
			t.Errorf("FirstOr empty = %d, want -1", got)
		}
		if got := LastOr(empty, -1); got != -1 {
			t.Errorf("LastOr empty = %d, want -1", got)
		}
	})
	t.Run("single element", func(t *testing.T) {
		s := []int{42}
		if v, _ := First(s); v != 42 {
			t.Errorf("First = %d", v)
		}
		if v, _ := Last(s); v != 42 {
			t.Errorf("Last = %d", v)
		}
	})
}

func BenchmarkEnsureIndex(b *testing.B) {
	b.Run("within len", func(b *testing.B) {
		s := make([]int, 100)
		for b.Loop() {
			_ = EnsureIndex(s, 50)
		}
	})
	b.Run("grow", func(b *testing.B) {
		for b.Loop() {
			_ = EnsureIndex([]int{1, 2, 3}, 100)
		}
	})
}

func BenchmarkChunk(b *testing.B) {
	s := make([]int, 1000)
	for b.Loop() {
		_ = Chunk(s, 10)
	}
}

func BenchmarkAt(b *testing.B) {
	s := []int{10, 20, 30, 40, 50}
	for b.Loop() {
		_, _ = At(s, -1)
	}
}
