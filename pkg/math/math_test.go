package math

import (
	"math"
	"testing"
)

// TestAbs_Integers tests Abs function with integer types
func TestAbs_Integers(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "int positive",
			test: func(t *testing.T) {
				got := Abs(5)
				want := 5
				if got != want {
					t.Errorf("Abs(5) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int negative",
			test: func(t *testing.T) {
				got := Abs(-5)
				want := 5
				if got != want {
					t.Errorf("Abs(-5) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int zero",
			test: func(t *testing.T) {
				got := Abs(0)
				want := 0
				if got != want {
					t.Errorf("Abs(0) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int64 positive",
			test: func(t *testing.T) {
				got := Abs(int64(12345))
				want := int64(12345)
				if got != want {
					t.Errorf("Abs(12345) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int64 negative",
			test: func(t *testing.T) {
				got := Abs(int64(-12345))
				want := int64(12345)
				if got != want {
					t.Errorf("Abs(-12345) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int32 negative",
			test: func(t *testing.T) {
				got := Abs(int32(-100))
				want := int32(100)
				if got != want {
					t.Errorf("Abs(-100) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int16 negative",
			test: func(t *testing.T) {
				got := Abs(int16(-50))
				want := int16(50)
				if got != want {
					t.Errorf("Abs(-50) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int8 negative",
			test: func(t *testing.T) {
				got := Abs(int8(-10))
				want := int8(10)
				if got != want {
					t.Errorf("Abs(-10) = %d, want %d", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestAbs_UnsignedIntegers tests Abs with unsigned integers
func TestAbs_UnsignedIntegers(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "uint",
			test: func(t *testing.T) {
				got := Abs(uint(10))
				want := uint(10)
				if got != want {
					t.Errorf("Abs(10) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "uint64",
			test: func(t *testing.T) {
				got := Abs(uint64(12345))
				want := uint64(12345)
				if got != want {
					t.Errorf("Abs(12345) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "uint32",
			test: func(t *testing.T) {
				got := Abs(uint32(100))
				want := uint32(100)
				if got != want {
					t.Errorf("Abs(100) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "uint16",
			test: func(t *testing.T) {
				got := Abs(uint16(50))
				want := uint16(50)
				if got != want {
					t.Errorf("Abs(50) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "uint8",
			test: func(t *testing.T) {
				got := Abs(uint8(25))
				want := uint8(25)
				if got != want {
					t.Errorf("Abs(25) = %d, want %d", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestAbs_Floats tests Abs function with floating point types
func TestAbs_Floats(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "float32 positive",
			test: func(t *testing.T) {
				got := Abs(float32(3.14))
				want := float32(3.14)
				if got != want {
					t.Errorf("Abs(3.14) = %f, want %f", got, want)
				}
			},
		},
		{
			name: "float32 negative",
			test: func(t *testing.T) {
				got := Abs(float32(-3.14))
				want := float32(3.14)
				if got != want {
					t.Errorf("Abs(-3.14) = %f, want %f", got, want)
				}
			},
		},
		{
			name: "float32 zero",
			test: func(t *testing.T) {
				got := Abs(float32(0.0))
				want := float32(0.0)
				if got != want {
					t.Errorf("Abs(0.0) = %f, want %f", got, want)
				}
			},
		},
		{
			name: "float64 positive",
			test: func(t *testing.T) {
				got := Abs(float64(2.71828))
				want := float64(2.71828)
				if got != want {
					t.Errorf("Abs(2.71828) = %f, want %f", got, want)
				}
			},
		},
		{
			name: "float64 negative",
			test: func(t *testing.T) {
				got := Abs(float64(-2.71828))
				want := float64(2.71828)
				if got != want {
					t.Errorf("Abs(-2.71828) = %f, want %f", got, want)
				}
			},
		},
		{
			name: "float64 negative zero",
			test: func(t *testing.T) {
				got := Abs(float64(-0.0))
				want := float64(0.0)
				if got != want {
					t.Errorf("Abs(-0.0) = %f, want %f", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestAbs_SpecialFloatValues tests Abs with special float values
func TestAbs_SpecialFloatValues(t *testing.T) {
	t.Run("positive infinity float64", func(t *testing.T) {
		got := Abs(math.Inf(1))
		if !math.IsInf(got, 1) {
			t.Errorf("Abs(+Inf) should be +Inf, got %f", got)
		}
	})

	t.Run("negative infinity float64", func(t *testing.T) {
		got := Abs(math.Inf(-1))
		if !math.IsInf(got, 1) {
			t.Errorf("Abs(-Inf) should be +Inf, got %f", got)
		}
	})

	t.Run("NaN float64", func(t *testing.T) {
		got := Abs(math.NaN())
		if !math.IsNaN(got) {
			t.Errorf("Abs(NaN) should be NaN, got %f", got)
		}
	})

	t.Run("positive infinity float32", func(t *testing.T) {
		got := Abs(float32(math.Inf(1)))
		if !math.IsInf(float64(got), 1) {
			t.Errorf("Abs(+Inf) should be +Inf, got %f", got)
		}
	})

	t.Run("negative infinity float32", func(t *testing.T) {
		got := Abs(float32(math.Inf(-1)))
		if !math.IsInf(float64(got), 1) {
			t.Errorf("Abs(-Inf) should be +Inf, got %f", got)
		}
	})
}

// TestAbs_EdgeCases tests edge cases for Abs
func TestAbs_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "int max value",
			test: func(t *testing.T) {
				got := Abs[int64](math.MaxInt64)
				want := int64(math.MaxInt64)
				if got != want {
					t.Errorf("Abs(MaxInt64) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "int8 max negative",
			test: func(t *testing.T) {
				got := Abs(int8(-127))
				want := int8(127)
				if got != want {
					t.Errorf("Abs(-127) = %d, want %d", got, want)
				}
			},
		},
		{
			name: "very small float",
			test: func(t *testing.T) {
				got := Abs(-1e-100)
				want := 1e-100
				if got != want {
					t.Errorf("Abs(-1e-100) = %e, want %e", got, want)
				}
			},
		},
		{
			name: "very large float",
			test: func(t *testing.T) {
				got := Abs(-1e100)
				want := 1e100
				if got != want {
					t.Errorf("Abs(-1e100) = %e, want %e", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestMultiplyExact_NoOverflow tests MultiplyExact without overflow
func TestMultiplyExact_NoOverflow(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		y    int64
		want int64
	}{
		{"zero multiplication", 0, 100, 0},
		{"multiply by one", 42, 1, 42},
		{"multiply by negative one", 42, -1, -42},
		{"small positive numbers", 10, 20, 200},
		{"small negative numbers", -10, -20, 200},
		{"mixed signs", -10, 20, -200},
		{"large safe multiplication", 1000000, 1000, 1000000000},
		{"negative result", -1000, 1000, -1000000},
		{"both negative", -100, -100, 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MultiplyExact(tt.x, tt.y)
			if err != nil {
				t.Errorf("MultiplyExact(%d, %d) unexpected error: %v", tt.x, tt.y, err)
				return
			}
			if got != tt.want {
				t.Errorf("MultiplyExact(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestMultiplyExact_Overflow tests MultiplyExact with overflow
func TestMultiplyExact_Overflow(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		y    int64
	}{
		{"max int64 overflow", math.MaxInt64, 2},
		{"min int64 special case", math.MinInt64, -1},
		{"large positive overflow", math.MaxInt64 / 2, 3},
		{"large negative overflow", math.MinInt64 / 2, 3},
		{"both max", math.MaxInt64, math.MaxInt64},
		{"max and min", math.MaxInt64, math.MinInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MultiplyExact(tt.x, tt.y)
			if err == nil {
				t.Errorf("MultiplyExact(%d, %d) expected overflow error, got nil", tt.x, tt.y)
			}
			if err != nil && err.Error() != "int64 overflow" {
				t.Errorf("MultiplyExact(%d, %d) error = %v, want 'int64 overflow'", tt.x, tt.y, err)
			}
		})
	}
}

// TestMultiplyExact_EdgeCases tests edge cases for MultiplyExact
func TestMultiplyExact_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		x       int64
		y       int64
		want    int64
		wantErr bool
	}{
		{"multiply by zero (x=0)", 0, math.MaxInt64, 0, false},
		{"multiply by zero (y=0)", math.MaxInt64, 0, 0, false},
		{"both zero", 0, 0, 0, false},
		{"one and max", 1, math.MaxInt64, math.MaxInt64, false},
		{"negative one and max", -1, math.MaxInt64, -math.MaxInt64, false},
		{"max safe multiplication", math.MaxInt64 / 2, 2, math.MaxInt64 - 1, false},
		{"min safe multiplication", math.MinInt64 / 2, 2, math.MinInt64, false},
		{"power of two", 1 << 30, 1 << 30, 1 << 60, false},
		{"near overflow boundary", 3037000499, 3037000499, 9223372030926249001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MultiplyExact(tt.x, tt.y)
			if (err != nil) != tt.wantErr {
				t.Errorf("MultiplyExact(%d, %d) error = %v, wantErr %v", tt.x, tt.y, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("MultiplyExact(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestMultiplyExact_Symmetry tests multiplication symmetry
func TestMultiplyExact_Symmetry(t *testing.T) {
	testCases := []struct {
		x int64
		y int64
	}{
		{10, 20},
		{-10, 20},
		{-10, -20},
		{100, 200},
		{1000000, 1000},
	}

	for _, tc := range testCases {
		t.Run("symmetry", func(t *testing.T) {
			result1, err1 := MultiplyExact(tc.x, tc.y)
			result2, err2 := MultiplyExact(tc.y, tc.x)

			if (err1 != nil) != (err2 != nil) {
				t.Errorf("error symmetry failed: MultiplyExact(%d, %d) err=%v, MultiplyExact(%d, %d) err=%v",
					tc.x, tc.y, err1, tc.y, tc.x, err2)
			}

			if err1 == nil && result1 != result2 {
				t.Errorf("result symmetry failed: MultiplyExact(%d, %d)=%d, MultiplyExact(%d, %d)=%d",
					tc.x, tc.y, result1, tc.y, tc.x, result2)
			}
		})
	}
}

// TestDivideExact_NoOverflow tests DivideExact without overflow
func TestDivideExact_NoOverflow(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		y    int64
		want int64
	}{
		{"simple division", 100, 10, 10},
		{"divide by one", 42, 1, 42},
		{"divide by negative one", 42, -1, -42},
		{"negative dividend", -100, 10, -10},
		{"negative divisor", 100, -10, -10},
		{"both negative", -100, -10, 10},
		{"exact division", 1000000, 1000, 1000},
		{"zero dividend", 0, 10, 0},
		{"large numbers", math.MaxInt64, 2, math.MaxInt64 / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DivideExact(tt.x, tt.y)
			if err != nil {
				t.Errorf("DivideExact(%d, %d) unexpected error: %v", tt.x, tt.y, err)
				return
			}
			if got != tt.want {
				t.Errorf("DivideExact(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestDivideExact_Overflow tests DivideExact with overflow
func TestDivideExact_Overflow(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		y    int64
	}{
		{"min int64 divided by -1", math.MinInt64, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DivideExact(tt.x, tt.y)
			if err == nil {
				t.Errorf("DivideExact(%d, %d) expected overflow error, got nil", tt.x, tt.y)
			}
			if err != nil && err.Error() != "int64 overflow" {
				t.Errorf("DivideExact(%d, %d) error = %v, want 'int64 overflow'", tt.x, tt.y, err)
			}
		})
	}
}

// TestDivideExact_EdgeCases tests edge cases for DivideExact
func TestDivideExact_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		x       int64
		y       int64
		want    int64
		wantErr bool
	}{
		{"max divided by max", math.MaxInt64, math.MaxInt64, 1, false},
		{"max divided by one", math.MaxInt64, 1, math.MaxInt64, false},
		{"max divided by negative one", math.MaxInt64, -1, -math.MaxInt64, false},
		{"min divided by one", math.MinInt64, 1, math.MinInt64, false},
		{"zero divided by positive", 0, 100, 0, false},
		{"zero divided by negative", 0, -100, 0, false},
		{"positive divided by max", 1000, math.MaxInt64, 0, false},
		{"min divided by max", math.MinInt64, math.MaxInt64, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DivideExact(tt.x, tt.y)
			if (err != nil) != tt.wantErr {
				t.Errorf("DivideExact(%d, %d) error = %v, wantErr %v", tt.x, tt.y, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("DivideExact(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestDivideExact_Truncation tests integer division truncation
func TestDivideExact_Truncation(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		y    int64
		want int64
	}{
		{"truncate positive", 10, 3, 3},
		{"truncate negative dividend", -10, 3, -3},
		{"truncate negative divisor", 10, -3, -3},
		{"truncate both negative", -10, -3, 3},
		{"no truncation needed", 15, 3, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DivideExact(tt.x, tt.y)
			if err != nil {
				t.Errorf("DivideExact(%d, %d) unexpected error: %v", tt.x, tt.y, err)
				return
			}
			if got != tt.want {
				t.Errorf("DivideExact(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// BenchmarkAbs benchmarks Abs function
func BenchmarkAbs(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Abs(-42)
		}
	})

	b.Run("int64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Abs(int64(-123456789))
		}
	})

	b.Run("float64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Abs(float64(-3.14159))
		}
	})

	b.Run("float32", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Abs(float32(-2.71828))
		}
	})
}

// BenchmarkMultiplyExact benchmarks MultiplyExact function
func BenchmarkMultiplyExact(b *testing.B) {
	b.Run("small numbers", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = MultiplyExact(10, 20)
		}
	})

	b.Run("large numbers", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = MultiplyExact(1000000, 1000000)
		}
	})

	b.Run("overflow case", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = MultiplyExact(math.MaxInt64, 2)
		}
	})
}

// BenchmarkDivideExact benchmarks DivideExact function
func BenchmarkDivideExact(b *testing.B) {
	b.Run("simple division", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = DivideExact(1000, 10)
		}
	})

	b.Run("large numbers", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = DivideExact(math.MaxInt64, 2)
		}
	})

	b.Run("overflow case", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = DivideExact(math.MinInt64, -1)
		}
	})
}
