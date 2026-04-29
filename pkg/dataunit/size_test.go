package dataunit

import (
	stdmath "math"
	"testing"
)

func TestConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int64
		want int64
	}{
		{"B", B, 1},
		{"KB", KB, 1024},
		{"MB", MB, 1 << 20},
		{"GB", GB, 1 << 30},
		{"TB", TB, 1 << 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDataSize_Int64(t *testing.T) {
	tests := []struct {
		in   DataSize
		want int64
	}{
		{0, 0},
		{1024, 1024},
		{-1024, -1024},
		{stdmath.MaxInt64, stdmath.MaxInt64},
		{stdmath.MinInt64, stdmath.MinInt64},
	}
	for _, tt := range tests {
		if got := tt.in.Int64(); got != tt.want {
			t.Errorf("Int64() = %d, want %d", got, tt.want)
		}
	}
}

func TestDataSize_Compare(t *testing.T) {
	tests := []struct {
		a, b DataSize
		want int
	}{
		{1024, 1024, 0},
		{512, 1024, -1},
		{2048, 1024, 1},
		{0, 0, 0},
		{-100, -50, -1},
		{-50, -100, 1},
		{-100, 100, -1},
	}
	for _, tt := range tests {
		if got := tt.a.Compare(tt.b); got != tt.want {
			t.Errorf("DataSize(%d).Compare(%d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDataSize_Sign(t *testing.T) {
	if !DataSize(-1).Negative() {
		t.Error("Negative -1")
	}
	if DataSize(0).Negative() {
		t.Error("Negative 0")
	}
	if !DataSize(1).Positive() {
		t.Error("Positive 1")
	}
	if DataSize(0).Positive() {
		t.Error("Positive 0")
	}
}

func TestDataSize_UnitConversions(t *testing.T) {
	s := DataSize(2 * GB)
	if s.B() != 2*GB {
		t.Errorf("B = %d, want %d", s.B(), 2*GB)
	}
	if s.KB() != 2*1024*1024 {
		t.Errorf("KB = %d", s.KB())
	}
	if s.MB() != 2*1024 {
		t.Errorf("MB = %d", s.MB())
	}
	if s.GB() != 2 {
		t.Errorf("GB = %d", s.GB())
	}
	if s.TB() != 0 {
		t.Errorf("TB = %d, want 0 (truncated)", s.TB())
	}
}

func TestSizeOf(t *testing.T) {
	if got := SizeOfB(123); got != 123 {
		t.Errorf("SizeOfB(123) = %d", got)
	}
	if got, err := SizeOfKB(2); err != nil || got != 2*KB {
		t.Errorf("SizeOfKB(2) = (%d, %v)", got, err)
	}
	if got, err := SizeOfMB(2); err != nil || got != 2*MB {
		t.Errorf("SizeOfMB(2) = (%d, %v)", got, err)
	}
	if got, err := SizeOfGB(2); err != nil || got != 2*GB {
		t.Errorf("SizeOfGB(2) = (%d, %v)", got, err)
	}
	if got, err := SizeOfTB(2); err != nil || got != 2*TB {
		t.Errorf("SizeOfTB(2) = (%d, %v)", got, err)
	}
}

func TestSizeOf_Overflow(t *testing.T) {
	if _, err := SizeOfTB(stdmath.MaxInt64); err == nil {
		t.Error("expected overflow error from SizeOfTB(MaxInt64)")
	}
	if _, err := SizeOfGB(stdmath.MaxInt64); err == nil {
		t.Error("expected overflow error from SizeOfGB(MaxInt64)")
	}
}

func BenchmarkSizeOfMB(b *testing.B) {
	for b.Loop() {
		_, _ = SizeOfMB(1024)
	}
}
