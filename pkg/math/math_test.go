package math

import (
	stdmath "math"
	"testing"
)

func TestAbs_Integers(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		for _, tc := range []struct{ in, want int }{
			{5, 5}, {-5, 5}, {0, 0}, {-1, 1},
		} {
			if got := Abs(tc.in); got != tc.want {
				t.Errorf("Abs(%d) = %d, want %d", tc.in, got, tc.want)
			}
		}
	})
	t.Run("typed integers", func(t *testing.T) {
		if got := Abs(int8(-10)); got != 10 {
			t.Errorf("Abs(int8 -10) = %d", got)
		}
		if got := Abs(int16(-50)); got != 50 {
			t.Errorf("Abs(int16 -50) = %d", got)
		}
		if got := Abs(int32(-100)); got != 100 {
			t.Errorf("Abs(int32 -100) = %d", got)
		}
		if got := Abs(int64(-12345)); got != 12345 {
			t.Errorf("Abs(int64 -12345) = %d", got)
		}
	})
	t.Run("unsigned passthrough", func(t *testing.T) {
		if got := Abs(uint(42)); got != 42 {
			t.Errorf("Abs(uint 42) = %d", got)
		}
	})
}

func TestAbs_Floats(t *testing.T) {
	if got := Abs(-3.14); got != 3.14 {
		t.Errorf("Abs(-3.14) = %f", got)
	}
	if got := Abs(float32(-3.14)); got != float32(3.14) {
		t.Errorf("Abs(float32 -3.14) = %f", got)
	}

	if got := Abs(stdmath.Inf(-1)); !stdmath.IsInf(got, 1) {
		t.Errorf("Abs(-Inf) = %f, want +Inf", got)
	}
	if got := Abs(stdmath.NaN()); !stdmath.IsNaN(got) {
		t.Errorf("Abs(NaN) = %f, want NaN", got)
	}
}

func TestAbs_SignedMin(t *testing.T) {
	// Documented behavior: Abs(MinInt) wraps and returns MinInt.
	if got := Abs[int64](stdmath.MinInt64); got != stdmath.MinInt64 {
		t.Errorf("Abs(MinInt64) = %d, want %d (wrap)", got, int64(stdmath.MinInt64))
	}
}

func TestMultiplyExact(t *testing.T) {
	tests := []struct {
		name    string
		x, y    int64
		want    int64
		wantErr bool
	}{
		{"small", 100, 200, 20000, false},
		{"zero x", 0, 1000, 0, false},
		{"zero y", 1000, 0, 0, false},
		{"negative", -10, 20, -200, false},
		{"max bound", stdmath.MaxInt32, 2, 2 * int64(stdmath.MaxInt32), false},
		{"overflow", stdmath.MaxInt64, 2, 0, true},
		{"min times -1", stdmath.MinInt64, -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MultiplyExact(tt.x, tt.y)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDivideExact(t *testing.T) {
	tests := []struct {
		name    string
		x, y    int64
		want    int64
		wantErr bool
	}{
		{"normal", 100, 5, 20, false},
		{"truncate", 7, 2, 3, false},
		{"negative", -10, 2, -5, false},
		{"min div -1 overflow", stdmath.MinInt64, -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DivideExact(tt.x, tt.y)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsNumericType(t *testing.T) {
	type myInt int
	tests := []struct {
		name string
		v    any
		want bool
	}{
		{"int", 1, true},
		{"int8", int8(1), true},
		{"uint64", uint64(1), true},
		{"float32", float32(1), true},
		{"float64", 1.0, true},
		{"string", "x", false},
		{"bool", true, false},
		{"nil", nil, false},
		{"slice", []int{}, false},
		{"defined int rejected", myInt(1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNumericType(tt.v); got != tt.want {
				t.Errorf("IsNumericType(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func BenchmarkAbs(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for b.Loop() {
			_ = Abs(-42)
		}
	})
	b.Run("float64", func(b *testing.B) {
		for b.Loop() {
			_ = Abs(-3.14)
		}
	})
}

func BenchmarkMultiplyExact(b *testing.B) {
	for b.Loop() {
		_, _ = MultiplyExact(1234, 5678)
	}
}
