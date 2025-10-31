package math

import (
	"math"
	"reflect"
	"testing"
)

// TestConvertSlice_IntToInt tests integer to integer conversions
func TestConvertSlice_IntToInt(t *testing.T) {
	tests := []struct {
		name   string
		source []int
		want   []int64
	}{
		{
			name:   "empty slice",
			source: []int{},
			want:   []int64{},
		},
		{
			name:   "single element",
			source: []int{42},
			want:   []int64{42},
		},
		{
			name:   "multiple elements",
			source: []int{1, 2, 3, 4, 5},
			want:   []int64{1, 2, 3, 4, 5},
		},
		{
			name:   "negative numbers",
			source: []int{-1, -10, -100},
			want:   []int64{-1, -10, -100},
		},
		{
			name:   "mixed positive and negative",
			source: []int{-5, 0, 5},
			want:   []int64{-5, 0, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[int, int64](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_IntToFloat tests integer to float conversions
func TestConvertSlice_IntToFloat(t *testing.T) {
	tests := []struct {
		name   string
		source []int
		want   []float64
	}{
		{
			name:   "empty slice",
			source: []int{},
			want:   []float64{},
		},
		{
			name:   "positive integers",
			source: []int{1, 2, 3},
			want:   []float64{1.0, 2.0, 3.0},
		},
		{
			name:   "negative integers",
			source: []int{-1, -2, -3},
			want:   []float64{-1.0, -2.0, -3.0},
		},
		{
			name:   "zero",
			source: []int{0},
			want:   []float64{0.0},
		},
		{
			name:   "large numbers",
			source: []int{1000000, 2000000},
			want:   []float64{1000000.0, 2000000.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[int, float64](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_FloatToInt tests float to integer conversions
func TestConvertSlice_FloatToInt(t *testing.T) {
	tests := []struct {
		name   string
		source []float64
		want   []int
	}{
		{
			name:   "empty slice",
			source: []float64{},
			want:   []int{},
		},
		{
			name:   "whole numbers",
			source: []float64{1.0, 2.0, 3.0},
			want:   []int{1, 2, 3},
		},
		{
			name:   "truncation",
			source: []float64{1.9, 2.5, 3.1},
			want:   []int{1, 2, 3},
		},
		{
			name:   "negative truncation",
			source: []float64{-1.9, -2.5, -3.1},
			want:   []int{-1, -2, -3},
		},
		{
			name:   "zero",
			source: []float64{0.0},
			want:   []int{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[float64, int](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_FloatToFloat tests float to float conversions
func TestConvertSlice_FloatToFloat(t *testing.T) {
	tests := []struct {
		name   string
		source []float32
		want   []float64
	}{
		{
			name:   "empty slice",
			source: []float32{},
			want:   []float64{},
		},
		{
			name:   "simple values",
			source: []float32{1.5, 2.5, 3.5},
			want:   []float64{1.5, 2.5, 3.5},
		},
		{
			name:   "negative values",
			source: []float32{-1.5, -2.5},
			want:   []float64{-1.5, -2.5},
		},
		{
			name:   "zero",
			source: []float32{0.0},
			want:   []float64{0.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[float32, float64](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_NilSlice tests nil slice handling
func TestConvertSlice_NilSlice(t *testing.T) {
	t.Run("nil int slice", func(t *testing.T) {
		var source []int
		got := ConvertSlice[int, float64](source)
		if got != nil {
			t.Errorf("ConvertSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("nil float slice", func(t *testing.T) {
		var source []float64
		got := ConvertSlice[float64, int](source)
		if got != nil {
			t.Errorf("ConvertSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("nil uint slice", func(t *testing.T) {
		var source []uint
		got := ConvertSlice[uint, int64](source)
		if got != nil {
			t.Errorf("ConvertSlice(nil) = %v, want nil", got)
		}
	})
}

// TestConvertSlice_UnsignedTypes tests unsigned integer conversions
func TestConvertSlice_UnsignedTypes(t *testing.T) {
	tests := []struct {
		name   string
		source []uint
		want   []uint64
	}{
		{
			name:   "empty slice",
			source: []uint{},
			want:   []uint64{},
		},
		{
			name:   "positive values",
			source: []uint{1, 2, 3},
			want:   []uint64{1, 2, 3},
		},
		{
			name:   "zero",
			source: []uint{0},
			want:   []uint64{0},
		},
		{
			name:   "large values",
			source: []uint{100000, 200000},
			want:   []uint64{100000, 200000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[uint, uint64](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_SignedToUnsigned tests signed to unsigned conversions
func TestConvertSlice_SignedToUnsigned(t *testing.T) {
	tests := []struct {
		name   string
		source []int
		want   []uint
	}{
		{
			name:   "positive values",
			source: []int{1, 2, 3},
			want:   []uint{1, 2, 3},
		},
		{
			name:   "zero",
			source: []int{0},
			want:   []uint{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSlice[int, uint](tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConvertSlice_SmallIntTypes tests conversions with int8, int16, int32
func TestConvertSlice_SmallIntTypes(t *testing.T) {
	t.Run("int8 to int64", func(t *testing.T) {
		source := []int8{1, -1, 127, -128}
		want := []int64{1, -1, 127, -128}
		got := ConvertSlice[int8, int64](source)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConvertSlice() = %v, want %v", got, want)
		}
	})

	t.Run("int16 to int32", func(t *testing.T) {
		source := []int16{1, -1, 32767, -32768}
		want := []int32{1, -1, 32767, -32768}
		got := ConvertSlice[int16, int32](source)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConvertSlice() = %v, want %v", got, want)
		}
	})

	t.Run("uint8 to uint16", func(t *testing.T) {
		source := []uint8{0, 1, 255}
		want := []uint16{0, 1, 255}
		got := ConvertSlice[uint8, uint16](source)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ConvertSlice() = %v, want %v", got, want)
		}
	})
}

// TestConvertSlice_Overflow tests overflow scenarios
func TestConvertSlice_Overflow(t *testing.T) {
	t.Run("int64 to int8 overflow", func(t *testing.T) {
		source := []int64{128, 256} // exceeds int8 max (127)
		got := ConvertSlice[int64, int8](source)
		// Values will wrap around due to overflow
		if len(got) != len(source) {
			t.Errorf("length mismatch: got %d, want %d", len(got), len(source))
		}
	})

	t.Run("uint64 to uint8 overflow", func(t *testing.T) {
		source := []uint64{256, 512} // exceeds uint8 max (255)
		got := ConvertSlice[uint64, uint8](source)
		// Values will wrap around due to overflow
		if len(got) != len(source) {
			t.Errorf("length mismatch: got %d, want %d", len(got), len(source))
		}
	})
}

// TestConvertSlice_PrecisionLoss tests precision loss in conversions
func TestConvertSlice_PrecisionLoss(t *testing.T) {
	t.Run("float64 to float32 precision loss", func(t *testing.T) {
		source := []float64{1.123456789012345}
		got := ConvertSlice[float64, float32](source)
		// float32 has less precision than float64
		if len(got) != 1 {
			t.Errorf("length mismatch: got %d, want 1", len(got))
		}
	})

	t.Run("large int to float32", func(t *testing.T) {
		source := []int64{9007199254740993} // exceeds float32 precision
		got := ConvertSlice[int64, float32](source)
		if len(got) != 1 {
			t.Errorf("length mismatch: got %d, want 1", len(got))
		}
	})
}

// TestConvertSlice_SpecialFloatValues tests special float values
func TestConvertSlice_SpecialFloatValues(t *testing.T) {
	t.Run("infinity values", func(t *testing.T) {
		source := []float64{math.Inf(1), math.Inf(-1)}
		got := ConvertSlice[float64, float32](source)
		if !math.IsInf(float64(got[0]), 1) {
			t.Error("positive infinity not preserved")
		}
		if !math.IsInf(float64(got[1]), -1) {
			t.Error("negative infinity not preserved")
		}
	})

	t.Run("NaN values", func(t *testing.T) {
		source := []float64{math.NaN()}
		got := ConvertSlice[float64, float32](source)
		if !math.IsNaN(float64(got[0])) {
			t.Error("NaN not preserved")
		}
	})

	t.Run("zero values", func(t *testing.T) {
		source := []float64{0.0, -0.0}
		got := ConvertSlice[float64, float32](source)
		if len(got) != 2 {
			t.Errorf("length mismatch: got %d, want 2", len(got))
		}
	})
}

// TestConvertSlice_LargeSlices tests performance with large slices
func TestConvertSlice_LargeSlices(t *testing.T) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		t.Run("size_"+string(rune(size)), func(t *testing.T) {
			source := make([]int, size)
			for i := range source {
				source[i] = i
			}

			got := ConvertSlice[int, float64](source)

			if len(got) != size {
				t.Errorf("length mismatch: got %d, want %d", len(got), size)
			}

			// Spot check some values
			if got[0] != 0.0 {
				t.Errorf("first element = %v, want 0.0", got[0])
			}
			if got[size-1] != float64(size-1) {
				t.Errorf("last element = %v, want %v", got[size-1], float64(size-1))
			}
		})
	}
}

// TestConvertSlice_AllIntegerTypes tests all integer type combinations
func TestConvertSlice_AllIntegerTypes(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "int to int32",
			test: func(t *testing.T) {
				source := []int{1, 2, 3}
				got := ConvertSlice[int, int32](source)
				want := []int32{1, 2, 3}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name: "uint32 to int64",
			test: func(t *testing.T) {
				source := []uint32{1, 2, 3}
				got := ConvertSlice[uint32, int64](source)
				want := []int64{1, 2, 3}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name: "int32 to uint64",
			test: func(t *testing.T) {
				source := []int32{1, 2, 3}
				got := ConvertSlice[int32, uint64](source)
				want := []uint64{1, 2, 3}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestConvertSlice_SameType tests converting to the same type
func TestConvertSlice_SameType(t *testing.T) {
	t.Run("int to int", func(t *testing.T) {
		source := []int{1, 2, 3}
		got := ConvertSlice[int, int](source)
		if !reflect.DeepEqual(got, source) {
			t.Errorf("ConvertSlice() = %v, want %v", got, source)
		}
	})

	t.Run("float64 to float64", func(t *testing.T) {
		source := []float64{1.5, 2.5, 3.5}
		got := ConvertSlice[float64, float64](source)
		if !reflect.DeepEqual(got, source) {
			t.Errorf("ConvertSlice() = %v, want %v", got, source)
		}
	})
}

// TestConvertSlice_CapacityCheck tests that result has correct capacity
func TestConvertSlice_CapacityCheck(t *testing.T) {
	source := []int{1, 2, 3, 4, 5}
	got := ConvertSlice[int, float64](source)

	if len(got) != len(source) {
		t.Errorf("length mismatch: got %d, want %d", len(got), len(source))
	}

	if cap(got) != len(source) {
		t.Errorf("capacity mismatch: got %d, want %d", cap(got), len(source))
	}
}

// BenchmarkConvertSlice benchmarks conversion performance
func BenchmarkConvertSlice(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		source := make([]int, size)
		for i := range source {
			source[i] = i
		}

		b.Run("int_to_float64_size_"+string(rune(size)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = ConvertSlice[int, float64](source)
			}
		})

		b.Run("int_to_int64_size_"+string(rune(size)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = ConvertSlice[int, int64](source)
			}
		})
	}
}

// BenchmarkConvertSlice_FloatToInt benchmarks float to int conversion
func BenchmarkConvertSlice_FloatToInt(b *testing.B) {
	source := make([]float64, 1000)
	for i := range source {
		source[i] = float64(i) + 0.5
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ConvertSlice[float64, int](source)
	}
}
