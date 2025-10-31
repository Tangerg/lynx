package dataunit

import (
	"math"
	"testing"
)

// TestConstants tests unit constants
func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    int64
		expected int64
	}{
		{"B", B, 1},
		{"KB", KB, 1024},
		{"MB", MB, 1024 * 1024},
		{"GB", GB, 1024 * 1024 * 1024},
		{"TB", TB, 1024 * 1024 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("constant %s = %d, want %d", tt.name, tt.value, tt.expected)
			}
		})
	}
}

// TestDataSize_Int64 tests Int64 method
func TestDataSize_Int64(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero value", DataSize(0), 0},
		{"positive number", DataSize(1024), 1024},
		{"negative number", DataSize(-1024), -1024},
		{"max int64", DataSize(math.MaxInt64), math.MaxInt64},
		{"min int64", DataSize(math.MinInt64), math.MinInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.Int64(); got != tt.want {
				t.Errorf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_Compare tests Compare method
func TestDataSize_Compare(t *testing.T) {
	tests := []struct {
		name  string
		size  DataSize
		other DataSize
		want  int
	}{
		{"equal", DataSize(1024), DataSize(1024), 0},
		{"less than", DataSize(512), DataSize(1024), -1},
		{"greater than", DataSize(2048), DataSize(1024), 1},
		{"zero comparison", DataSize(0), DataSize(0), 0},
		{"negative comparison less", DataSize(-100), DataSize(-50), -1},
		{"negative comparison greater", DataSize(-50), DataSize(-100), 1},
		{"negative vs positive", DataSize(-100), DataSize(100), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.Compare(tt.other); got != tt.want {
				t.Errorf("Compare() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_Negative tests Negative method
func TestDataSize_Negative(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want bool
	}{
		{"negative number", DataSize(-1), true},
		{"zero", DataSize(0), false},
		{"positive number", DataSize(1), false},
		{"large negative", DataSize(-1024000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.Negative(); got != tt.want {
				t.Errorf("Negative() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDataSize_Positive tests Positive method
func TestDataSize_Positive(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want bool
	}{
		{"positive number", DataSize(1), true},
		{"zero", DataSize(0), false},
		{"negative number", DataSize(-1), false},
		{"large positive", DataSize(1024000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.Positive(); got != tt.want {
				t.Errorf("Positive() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDataSize_B tests B method
func TestDataSize_B(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero", DataSize(0), 0},
		{"1 KB in bytes", DataSize(1024), 1024},
		{"1 MB in bytes", DataSize(1048576), 1048576},
		{"negative", DataSize(-1024), -1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.B(); got != tt.want {
				t.Errorf("B() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_KB tests KB method
func TestDataSize_KB(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero", DataSize(0), 0},
		{"1 KB", DataSize(1024), 1},
		{"1.5 KB rounded down", DataSize(1536), 1},
		{"2 KB", DataSize(2048), 2},
		{"1 MB", DataSize(1048576), 1024},
		{"negative", DataSize(-1024), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.KB(); got != tt.want {
				t.Errorf("KB() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_MB tests MB method
func TestDataSize_MB(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero", DataSize(0), 0},
		{"1 MB", DataSize(1048576), 1},
		{"1.5 MB rounded down", DataSize(1572864), 1},
		{"2 MB", DataSize(2097152), 2},
		{"1 GB", DataSize(1073741824), 1024},
		{"negative", DataSize(-1048576), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.MB(); got != tt.want {
				t.Errorf("MB() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_GB tests GB method
func TestDataSize_GB(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero", DataSize(0), 0},
		{"1 GB", DataSize(1073741824), 1},
		{"1.5 GB rounded down", DataSize(1610612736), 1},
		{"2 GB", DataSize(2147483648), 2},
		{"1 TB", DataSize(1099511627776), 1024},
		{"negative", DataSize(-1073741824), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.GB(); got != tt.want {
				t.Errorf("GB() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDataSize_TB tests TB method
func TestDataSize_TB(t *testing.T) {
	tests := []struct {
		name string
		size DataSize
		want int64
	}{
		{"zero", DataSize(0), 0},
		{"1 TB", DataSize(1099511627776), 1},
		{"1.5 TB rounded down", DataSize(1649267441664), 1},
		{"2 TB", DataSize(2199023255552), 2},
		{"negative", DataSize(-1099511627776), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.TB(); got != tt.want {
				t.Errorf("TB() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSizeOfB tests SizeOfB function
func TestSizeOfB(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  DataSize
	}{
		{"zero", 0, DataSize(0)},
		{"positive", 1024, DataSize(1024)},
		{"negative", -1024, DataSize(-1024)},
		{"max int64", math.MaxInt64, DataSize(math.MaxInt64)},
		{"min int64", math.MinInt64, DataSize(math.MinInt64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SizeOfB(tt.bytes); got != tt.want {
				t.Errorf("SizeOfB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSizeOfKB tests SizeOfKB function
func TestSizeOfKB(t *testing.T) {
	tests := []struct {
		name    string
		kb      int64
		want    DataSize
		wantErr bool
	}{
		{"zero", 0, DataSize(0), false},
		{"1 KB", 1, DataSize(1024), false},
		{"100 KB", 100, DataSize(102400), false},
		{"negative", -1, DataSize(-1024), false},
		{"overflow", math.MaxInt64/KB + 1, DataSize(0), true},
		{"max valid", math.MaxInt64 / KB, DataSize(math.MaxInt64 / KB * KB), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SizeOfKB(tt.kb)
			if (err != nil) != tt.wantErr {
				t.Errorf("SizeOfKB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SizeOfKB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSizeOfMB tests SizeOfMB function
func TestSizeOfMB(t *testing.T) {
	tests := []struct {
		name    string
		mb      int64
		want    DataSize
		wantErr bool
	}{
		{"zero", 0, DataSize(0), false},
		{"1 MB", 1, DataSize(1048576), false},
		{"100 MB", 100, DataSize(104857600), false},
		{"negative", -1, DataSize(-1048576), false},
		{"overflow", math.MaxInt64/MB + 1, DataSize(0), true},
		{"max valid", math.MaxInt64 / MB, DataSize(math.MaxInt64 / MB * MB), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SizeOfMB(tt.mb)
			if (err != nil) != tt.wantErr {
				t.Errorf("SizeOfMB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SizeOfMB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSizeOfGB tests SizeOfGB function
func TestSizeOfGB(t *testing.T) {
	tests := []struct {
		name    string
		gb      int64
		want    DataSize
		wantErr bool
	}{
		{"zero", 0, DataSize(0), false},
		{"1 GB", 1, DataSize(1073741824), false},
		{"10 GB", 10, DataSize(10737418240), false},
		{"negative", -1, DataSize(-1073741824), false},
		{"overflow", math.MaxInt64/GB + 1, DataSize(0), true},
		{"max valid", math.MaxInt64 / GB, DataSize(math.MaxInt64 / GB * GB), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SizeOfGB(tt.gb)
			if (err != nil) != tt.wantErr {
				t.Errorf("SizeOfGB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SizeOfGB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSizeOfTB tests SizeOfTB function
func TestSizeOfTB(t *testing.T) {
	tests := []struct {
		name    string
		tb      int64
		want    DataSize
		wantErr bool
	}{
		{"zero", 0, DataSize(0), false},
		{"1 TB", 1, DataSize(1099511627776), false},
		{"5 TB", 5, DataSize(5497558138880), false},
		{"negative", -1, DataSize(-1099511627776), false},
		{"overflow", math.MaxInt64/TB + 1, DataSize(0), true},
		{"max valid", math.MaxInt64 / TB, DataSize(math.MaxInt64 / TB * TB), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SizeOfTB(tt.tb)
			if (err != nil) != tt.wantErr {
				t.Errorf("SizeOfTB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SizeOfTB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDataSize_ConversionChain tests conversion chain consistency
func TestDataSize_ConversionChain(t *testing.T) {
	// Create a size from GB and verify all conversions
	size, err := SizeOfGB(5)
	if err != nil {
		t.Fatalf("SizeOfGB() error = %v", err)
	}

	tests := []struct {
		name string
		got  int64
		want int64
	}{
		{"bytes", size.B(), 5 * GB},
		{"kilobytes", size.KB(), 5 * GB / KB},
		{"megabytes", size.MB(), 5 * GB / MB},
		{"gigabytes", size.GB(), 5},
		{"terabytes", size.TB(), 0}, // 5GB < 1TB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("conversion %s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestDataSize_EdgeCases tests edge cases
func TestDataSize_EdgeCases(t *testing.T) {
	t.Run("max int64 conversions", func(t *testing.T) {
		size := DataSize(math.MaxInt64)

		// These should not panic
		_ = size.B()
		_ = size.KB()
		_ = size.MB()
		_ = size.GB()
		_ = size.TB()
	})

	t.Run("min int64 conversions", func(t *testing.T) {
		size := DataSize(math.MinInt64)

		// These should not panic
		_ = size.B()
		_ = size.KB()
		_ = size.MB()
		_ = size.GB()
		_ = size.TB()
	})

	t.Run("comparison with self", func(t *testing.T) {
		size := DataSize(1024)
		if got := size.Compare(size); got != 0 {
			t.Errorf("self comparison should return 0, got %d", got)
		}
	})
}
