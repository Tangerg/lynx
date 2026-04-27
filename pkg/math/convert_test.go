package math

import (
	"reflect"
	"testing"
)

func TestConvertSlice_NilAndEmpty(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if got := ConvertSlice[int, float64](nil); got != nil {
			t.Errorf("ConvertSlice(nil) = %v, want nil", got)
		}
	})
	t.Run("empty returns empty non-nil", func(t *testing.T) {
		got := ConvertSlice[int, float64]([]int{})
		if got == nil || len(got) != 0 {
			t.Errorf("ConvertSlice([]) = %v, want []", got)
		}
	})
}

func TestConvertSlice_Basic(t *testing.T) {
	t.Run("int to float64", func(t *testing.T) {
		got := ConvertSlice[int, float64]([]int{1, 2, 3})
		want := []float64{1.0, 2.0, 3.0}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("float64 to float32 truncates precision", func(t *testing.T) {
		got := ConvertSlice[float64, float32]([]float64{1.5, 2.5})
		want := []float32{1.5, 2.5}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("float to int truncates", func(t *testing.T) {
		got := ConvertSlice[float64, int]([]float64{1.9, 2.1, -3.7})
		want := []int{1, 2, -3}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("int64 to int8 wraps", func(t *testing.T) {
		got := ConvertSlice[int64, int8]([]int64{127, 128, 129})
		// 128 → -128, 129 → -127 by Go conversion rules.
		want := []int8{127, -128, -127}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestConvertSlice_DefinedTypes(t *testing.T) {
	type myInt int
	type myFloat float64
	got := ConvertSlice[myInt, myFloat]([]myInt{1, 2, 3})
	want := []myFloat{1.0, 2.0, 3.0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func BenchmarkConvertSlice(b *testing.B) {
	src := make([]float64, 1000)
	for i := range src {
		src[i] = float64(i)
	}
	for b.Loop() {
		_ = ConvertSlice[float64, float32](src)
	}
}
