package random

import (
	"testing"
)

// TestIntRange_BasicFunctionality tests basic random number generation
func TestIntRange_BasicFunctionality(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"small positive range", 0, 10},
		{"large positive range", 0, 1000},
		{"positive range with offset", 10, 20},
		{"single value range", 5, 6},
		{"large offset range", 100, 200},
		{"very large range", 0, 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to test randomness
			for i := 0; i < 100; i++ {
				got := IntRange(tt.min, tt.max)

				if got < tt.min {
					t.Errorf("IntRange(%d, %d) = %d, which is less than min %d",
						tt.min, tt.max, got, tt.min)
				}

				if got >= tt.max {
					t.Errorf("IntRange(%d, %d) = %d, which is >= max %d",
						tt.min, tt.max, got, tt.max)
				}
			}
		})
	}
}

// TestIntRange_BoundaryValues tests boundary conditions
func TestIntRange_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"zero to one", 0, 1},
		{"one to two", 1, 2},
		{"zero to hundred", 0, 100},
		{"negative to positive", -10, 10},
		{"both negative", -20, -10},
		{"large negative to zero", -100, 0},
		{"large negative range", -1000, -900},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 50; i++ {
				got := IntRange(tt.min, tt.max)

				if got < tt.min || got >= tt.max {
					t.Errorf("IntRange(%d, %d) = %d, out of range [%d, %d)",
						tt.min, tt.max, got, tt.min, tt.max)
				}
			}
		})
	}
}

// TestIntRange_SingleValueRange tests when max = min + 1
func TestIntRange_SingleValueRange(t *testing.T) {
	tests := []struct {
		name     string
		min      int
		max      int
		expected int
	}{
		{"zero to one", 0, 1, 0},
		{"five to six", 5, 6, 5},
		{"negative", -1, 0, -1},
		{"large value", 999, 1000, 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When range has only one value, should always return that value
			for i := 0; i < 10; i++ {
				got := IntRange(tt.min, tt.max)
				if got != tt.expected {
					t.Errorf("IntRange(%d, %d) = %d, want %d",
						tt.min, tt.max, got, tt.expected)
				}
			}
		})
	}
}

// TestIntRange_Distribution tests that all values in range can be generated
func TestIntRange_Distribution(t *testing.T) {
	t.Run("small range distribution", func(t *testing.T) {
		min, max := 0, 5
		iterations := 1000
		counts := make(map[int]int)

		for i := 0; i < iterations; i++ {
			got := IntRange(min, max)
			counts[got]++
		}

		// Check that all possible values were generated
		for i := min; i < max; i++ {
			if counts[i] == 0 {
				t.Errorf("value %d was never generated in range [%d, %d)",
					i, min, max)
			}
		}

		// Check that no values outside range were generated
		for val := range counts {
			if val < min || val >= max {
				t.Errorf("invalid value %d generated outside range [%d, %d)",
					val, min, max)
			}
		}
	})

	t.Run("negative range distribution", func(t *testing.T) {
		min, max := -3, 3
		iterations := 1000
		counts := make(map[int]int)

		for i := 0; i < iterations; i++ {
			got := IntRange(min, max)
			counts[got]++
		}

		// Check all values in range were generated
		for i := min; i < max; i++ {
			if counts[i] == 0 {
				t.Errorf("value %d was never generated in range [%d, %d)",
					i, min, max)
			}
		}
	})
}

// TestIntRange_Panic_MinGreaterThanMax tests panic when min > max
func TestIntRange_Panic_MinGreaterThanMax(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"simple reverse", 10, 5},
		{"large reverse", 1000, 100},
		{"negative reverse", -5, -10},
		{"negative to positive reverse", 10, -10},
		{"by one", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("IntRange(%d, %d) should panic when min > max",
						tt.min, tt.max)
				} else {
					// Verify panic message
					if msg, ok := r.(string); ok {
						expected := "IntRange: min cannot be greater than max"
						if msg != expected {
							t.Errorf("panic message = %q, want %q", msg, expected)
						}
					}
				}
			}()

			_ = IntRange(tt.min, tt.max)
		})
	}
}

// TestIntRange_Panic_MaxLessThanOrEqualZero tests panic when max <= 0 and min >= 0
func TestIntRange_Panic_MaxLessThanOrEqualZero(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"max is zero", 0, 0},
		{"max is negative", 0, -5},
		{"both zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This tests the documented behavior that rand.IntN panics if max <= 0
			// Since max-min <= 0 when min >= max, this should panic
			defer func() {
				if r := recover(); r == nil {
					// Expected to panic due to either min > max check or rand.IntN
					t.Errorf("IntRange(%d, %d) should panic", tt.min, tt.max)
				}
			}()

			_ = IntRange(tt.min, tt.max)
		})
	}
}

// TestIntRange_NegativeRanges tests negative number ranges
func TestIntRange_NegativeRanges(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"both negative", -10, -5},
		{"negative to zero", -10, 0},
		{"negative to positive", -10, 10},
		{"large negative", -1000, -500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				got := IntRange(tt.min, tt.max)

				if got < tt.min {
					t.Errorf("IntRange(%d, %d) = %d, less than min",
						tt.min, tt.max, got)
				}

				if got >= tt.max {
					t.Errorf("IntRange(%d, %d) = %d, >= max",
						tt.min, tt.max, got)
				}
			}
		})
	}
}

// TestIntRange_LargeRanges tests with large ranges
func TestIntRange_LargeRanges(t *testing.T) {
	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"million range", 0, 1000000},
		{"large offset", 1000000, 2000000},
		{"negative large", -1000000, 0},
		{"symmetric large", -500000, 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test fewer iterations for large ranges
			for i := 0; i < 10; i++ {
				got := IntRange(tt.min, tt.max)

				if got < tt.min || got >= tt.max {
					t.Errorf("IntRange(%d, %d) = %d, out of range",
						tt.min, tt.max, got)
				}
			}
		})
	}
}

// TestIntRange_ConsecutiveRanges tests consecutive value ranges
func TestIntRange_ConsecutiveRanges(t *testing.T) {
	ranges := []struct {
		min int
		max int
	}{
		{0, 10},
		{10, 20},
		{20, 30},
		{-10, 0},
		{-20, -10},
	}

	for _, r := range ranges {
		t.Run("range", func(t *testing.T) {
			for i := 0; i < 50; i++ {
				got := IntRange(r.min, r.max)
				if got < r.min || got >= r.max {
					t.Errorf("IntRange(%d, %d) = %d, out of range",
						r.min, r.max, got)
				}
			}
		})
	}
}

// TestIntRange_Randomness tests basic randomness properties
func TestIntRange_Randomness(t *testing.T) {
	t.Run("consecutive calls produce different values", func(t *testing.T) {
		min, max := 0, 100
		iterations := 20
		values := make([]int, iterations)

		for i := 0; i < iterations; i++ {
			values[i] = IntRange(min, max)
		}

		// Check that not all values are the same (very unlikely with proper randomness)
		allSame := true
		first := values[0]
		for _, v := range values[1:] {
			if v != first {
				allSame = false
				break
			}
		}

		if allSame {
			t.Error("all generated values are identical, randomness may be broken")
		}
	})

	t.Run("values spread across range", func(t *testing.T) {
		min, max := 0, 10
		iterations := 100
		counts := make(map[int]int)

		for i := 0; i < iterations; i++ {
			got := IntRange(min, max)
			counts[got]++
		}

		// With 100 iterations and 10 possible values,
		// we expect some diversity (not all values in one bucket)
		if len(counts) < 3 {
			t.Errorf("only %d unique values generated out of %d possible, randomness may be poor",
				len(counts), max-min)
		}
	})
}

// TestIntRange_MinEqualsMax tests edge case where min equals max
func TestIntRange_MinEqualsMax(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"positive", 10},
		{"negative", -5},
		{"large", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("IntRange(%d, %d) should panic when min == max",
						tt.value, tt.value)
				}
			}()

			_ = IntRange(tt.value, tt.value)
		})
	}
}

// TestIntRange_SpecificRanges tests specific edge case ranges
func TestIntRange_SpecificRanges(t *testing.T) {
	t.Run("range with min 0", func(t *testing.T) {
		min, max := 0, 100
		for i := 0; i < 50; i++ {
			got := IntRange(min, max)
			if got < 0 || got >= 100 {
				t.Errorf("IntRange(0, 100) = %d, out of range", got)
			}
		}
	})

	t.Run("range with max 0", func(t *testing.T) {
		min, max := -100, 0
		for i := 0; i < 50; i++ {
			got := IntRange(min, max)
			if got < -100 || got >= 0 {
				t.Errorf("IntRange(-100, 0) = %d, out of range", got)
			}
		}
	})

	t.Run("symmetric range around zero", func(t *testing.T) {
		min, max := -50, 50
		for i := 0; i < 50; i++ {
			got := IntRange(min, max)
			if got < -50 || got >= 50 {
				t.Errorf("IntRange(-50, 50) = %d, out of range", got)
			}
		}
	})
}

// BenchmarkIntRange benchmarks the IntRange function
func BenchmarkIntRange(b *testing.B) {
	b.Run("small range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IntRange(0, 10)
		}
	})

	b.Run("medium range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IntRange(0, 1000)
		}
	})

	b.Run("large range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IntRange(0, 1000000)
		}
	})

	b.Run("negative range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IntRange(-1000, 1000)
		}
	})

	b.Run("offset range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IntRange(1000, 2000)
		}
	})
}

// BenchmarkIntRange_SingleValue benchmarks single value range
func BenchmarkIntRange_SingleValue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = IntRange(5, 6)
	}
}

// TestIntRange_Concurrency tests concurrent usage
func TestIntRange_Concurrency(t *testing.T) {
	t.Run("concurrent calls", func(t *testing.T) {
		iterations := 100
		goroutines := 10
		done := make(chan bool, goroutines)

		for g := 0; g < goroutines; g++ {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("panic in goroutine: %v", r)
					}
					done <- true
				}()

				for i := 0; i < iterations; i++ {
					got := IntRange(0, 100)
					if got < 0 || got >= 100 {
						t.Errorf("invalid value %d from concurrent call", got)
					}
				}
			}()
		}

		for g := 0; g < goroutines; g++ {
			<-done
		}
	})
}
