package dataunit

import (
	"testing"
)

// TestDataUnitSuffixConstants tests suffix constants
func TestDataUnitSuffixConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"B suffix", BSuffix, "B"},
		{"KB suffix", KBSuffix, "KB"},
		{"MB suffix", MBSuffix, "MB"},
		{"GB suffix", GBSuffix, "GB"},
		{"TB suffix", TBSuffix, "TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("constant = %s, want %s", tt.constant, tt.expected)
			}
		})
	}
}

// TestDataUnit_Size tests Size method
func TestDataUnit_Size(t *testing.T) {
	tests := []struct {
		name string
		unit *DataUnit
		want DataSize
	}{
		{
			name: "byte unit",
			unit: &DataUnit{size: DataSize(1), suffix: BSuffix},
			want: DataSize(1),
		},
		{
			name: "kilobyte unit",
			unit: &DataUnit{size: DataSize(1024), suffix: KBSuffix},
			want: DataSize(1024),
		},
		{
			name: "megabyte unit",
			unit: &DataUnit{size: DataSize(1048576), suffix: MBSuffix},
			want: DataSize(1048576),
		},
		{
			name: "zero size",
			unit: &DataUnit{size: DataSize(0), suffix: BSuffix},
			want: DataSize(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.unit.Size(); got != tt.want {
				t.Errorf("Size() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDataUnit_Suffix tests Suffix method
func TestDataUnit_Suffix(t *testing.T) {
	tests := []struct {
		name string
		unit *DataUnit
		want string
	}{
		{
			name: "B suffix",
			unit: &DataUnit{size: DataSize(1), suffix: BSuffix},
			want: BSuffix,
		},
		{
			name: "KB suffix",
			unit: &DataUnit{size: DataSize(1024), suffix: KBSuffix},
			want: KBSuffix,
		},
		{
			name: "MB suffix",
			unit: &DataUnit{size: DataSize(1048576), suffix: MBSuffix},
			want: MBSuffix,
		},
		{
			name: "GB suffix",
			unit: &DataUnit{size: DataSize(1073741824), suffix: GBSuffix},
			want: GBSuffix,
		},
		{
			name: "TB suffix",
			unit: &DataUnit{size: DataSize(1099511627776), suffix: TBSuffix},
			want: TBSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.unit.Suffix(); got != tt.want {
				t.Errorf("Suffix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewUnitFromSuffix tests NewUnitFromSuffix function with valid suffixes
func TestNewUnitFromSuffix(t *testing.T) {
	tests := []struct {
		name       string
		suffix     string
		wantSize   DataSize
		wantSuffix string
		wantErr    bool
	}{
		{
			name:       "B suffix uppercase",
			suffix:     "B",
			wantSize:   DataSize(1),
			wantSuffix: BSuffix,
			wantErr:    false,
		},
		{
			name:       "B suffix lowercase",
			suffix:     "b",
			wantSize:   DataSize(1),
			wantSuffix: BSuffix,
			wantErr:    false,
		},
		{
			name:       "KB suffix uppercase",
			suffix:     "KB",
			wantSize:   DataSize(1024),
			wantSuffix: KBSuffix,
			wantErr:    false,
		},
		{
			name:       "KB suffix lowercase",
			suffix:     "kb",
			wantSize:   DataSize(1024),
			wantSuffix: KBSuffix,
			wantErr:    false,
		},
		{
			name:       "KB suffix mixed case",
			suffix:     "Kb",
			wantSize:   DataSize(1024),
			wantSuffix: KBSuffix,
			wantErr:    false,
		},
		{
			name:       "MB suffix uppercase",
			suffix:     "MB",
			wantSize:   DataSize(1048576),
			wantSuffix: MBSuffix,
			wantErr:    false,
		},
		{
			name:       "MB suffix lowercase",
			suffix:     "mb",
			wantSize:   DataSize(1048576),
			wantSuffix: MBSuffix,
			wantErr:    false,
		},
		{
			name:       "GB suffix uppercase",
			suffix:     "GB",
			wantSize:   DataSize(1073741824),
			wantSuffix: GBSuffix,
			wantErr:    false,
		},
		{
			name:       "GB suffix lowercase",
			suffix:     "gb",
			wantSize:   DataSize(1073741824),
			wantSuffix: GBSuffix,
			wantErr:    false,
		},
		{
			name:       "TB suffix uppercase",
			suffix:     "TB",
			wantSize:   DataSize(1099511627776),
			wantSuffix: TBSuffix,
			wantErr:    false,
		},
		{
			name:       "TB suffix lowercase",
			suffix:     "tb",
			wantSize:   DataSize(1099511627776),
			wantSuffix: TBSuffix,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewUnitFromSuffix(tt.suffix)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewUnitFromSuffix() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Fatal("NewUnitFromSuffix() returned nil")
				}
				if got.Size() != tt.wantSize {
					t.Errorf("Size() = %v, want %v", got.Size(), tt.wantSize)
				}
				if got.Suffix() != tt.wantSuffix {
					t.Errorf("Suffix() = %v, want %v", got.Suffix(), tt.wantSuffix)
				}
			}
		})
	}
}

// TestNewUnitFromSuffix_InvalidSuffix tests NewUnitFromSuffix with invalid suffixes
func TestNewUnitFromSuffix_InvalidSuffix(t *testing.T) {
	tests := []struct {
		name   string
		suffix string
	}{
		{"empty string", ""},
		{"unknown suffix", "PB"},
		{"invalid suffix", "XYZ"},
		{"numeric suffix", "123"},
		{"special characters", "@#$"},
		{"space", " "},
		{"multiple spaces", "  "},
		{"suffix with spaces", "K B"},
		{"bytes spelled out", "BYTES"},
		{"kilobytes spelled out", "KILOBYTES"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewUnitFromSuffix(tt.suffix)
			if err == nil {
				t.Errorf("NewUnitFromSuffix() expected error for suffix %q, but got nil", tt.suffix)
			}
			if got != nil {
				t.Errorf("NewUnitFromSuffix() expected nil result for invalid suffix, got %v", got)
			}
		})
	}
}

// TestNewUnitFromSuffix_ErrorMessage tests error message format
func TestNewUnitFromSuffix_ErrorMessage(t *testing.T) {
	invalidSuffix := "INVALID"
	_, err := NewUnitFromSuffix(invalidSuffix)

	if err == nil {
		t.Fatal("expected error for invalid suffix")
	}

	expectedMsg := "unknown data unit suffix: INVALID"
	if err.Error() != expectedMsg {
		t.Errorf("error message = %q, want %q", err.Error(), expectedMsg)
	}
}

// TestDataUnit_Consistency tests consistency between created units
func TestDataUnit_Consistency(t *testing.T) {
	tests := []struct {
		name   string
		suffix string
	}{
		{"B unit", BSuffix},
		{"KB unit", KBSuffix},
		{"MB unit", MBSuffix},
		{"GB unit", GBSuffix},
		{"TB unit", TBSuffix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unit1, err1 := NewUnitFromSuffix(tt.suffix)
			unit2, err2 := NewUnitFromSuffix(tt.suffix)

			if err1 != nil || err2 != nil {
				t.Fatalf("unexpected error creating units: %v, %v", err1, err2)
			}

			if unit1.Size() != unit2.Size() {
				t.Errorf("sizes are inconsistent: %v != %v", unit1.Size(), unit2.Size())
			}

			if unit1.Suffix() != unit2.Suffix() {
				t.Errorf("suffixes are inconsistent: %v != %v", unit1.Suffix(), unit2.Suffix())
			}
		})
	}
}

// TestDataUnit_SizeRelationships tests relationships between different units
func TestDataUnit_SizeRelationships(t *testing.T) {
	bUnit, _ := NewUnitFromSuffix(BSuffix)
	kbUnit, _ := NewUnitFromSuffix(KBSuffix)
	mbUnit, _ := NewUnitFromSuffix(MBSuffix)
	gbUnit, _ := NewUnitFromSuffix(GBSuffix)
	tbUnit, _ := NewUnitFromSuffix(TBSuffix)

	tests := []struct {
		name    string
		smaller *DataUnit
		larger  *DataUnit
		ratio   int64
	}{
		{"B to KB", bUnit, kbUnit, 1024},
		{"KB to MB", kbUnit, mbUnit, 1024},
		{"MB to GB", mbUnit, gbUnit, 1024},
		{"GB to TB", gbUnit, tbUnit, 1024},
		{"B to MB", bUnit, mbUnit, 1024 * 1024},
		{"B to GB", bUnit, gbUnit, 1024 * 1024 * 1024},
		{"KB to GB", kbUnit, gbUnit, 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.larger.Size()/tt.smaller.Size() != DataSize(tt.ratio) {
				t.Errorf("size ratio = %v, want %v",
					tt.larger.Size()/tt.smaller.Size(), tt.ratio)
			}
		})
	}
}

// TestDataUnit_NilPointer tests behavior with nil pointer
func TestDataUnit_NilPointer(t *testing.T) {
	var unit *DataUnit

	// These should panic, but we test defensive behavior if needed
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling Size() on nil DataUnit")
		}
	}()

	_ = unit.Size()
}

// TestDataUnit_CaseInsensitivity tests case insensitivity thoroughly
func TestDataUnit_CaseInsensitivity(t *testing.T) {
	cases := []string{"B", "b"}
	var units []*DataUnit

	for _, c := range cases {
		unit, err := NewUnitFromSuffix(c)
		if err != nil {
			t.Fatalf("NewUnitFromSuffix(%q) failed: %v", c, err)
		}
		units = append(units, unit)
	}

	// All units should have the same size and suffix
	firstUnit := units[0]
	for i, unit := range units[1:] {
		if unit.Size() != firstUnit.Size() {
			t.Errorf("unit[%d].Size() = %v, want %v", i+1, unit.Size(), firstUnit.Size())
		}
		if unit.Suffix() != firstUnit.Suffix() {
			t.Errorf("unit[%d].Suffix() = %v, want %v", i+1, unit.Suffix(), firstUnit.Suffix())
		}
	}
}

// TestDataUnit_AllUnitsCreatable tests that all standard units can be created
func TestDataUnit_AllUnitsCreatable(t *testing.T) {
	suffixes := []string{BSuffix, KBSuffix, MBSuffix, GBSuffix, TBSuffix}

	for _, suffix := range suffixes {
		t.Run(suffix, func(t *testing.T) {
			unit, err := NewUnitFromSuffix(suffix)
			if err != nil {
				t.Errorf("NewUnitFromSuffix(%q) failed: %v", suffix, err)
			}
			if unit == nil {
				t.Errorf("NewUnitFromSuffix(%q) returned nil without error", suffix)
			}
			if unit != nil && unit.Suffix() != suffix {
				t.Errorf("unit.Suffix() = %q, want %q", unit.Suffix(), suffix)
			}
		})
	}
}

// BenchmarkNewUnitFromSuffix benchmarks unit creation
func BenchmarkNewUnitFromSuffix(b *testing.B) {
	suffixes := []string{"B", "KB", "MB", "GB", "TB"}

	for _, suffix := range suffixes {
		b.Run(suffix, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = NewUnitFromSuffix(suffix)
			}
		})
	}
}

// BenchmarkNewUnitFromSuffix_CaseConversion benchmarks with lowercase input
func BenchmarkNewUnitFromSuffix_CaseConversion(b *testing.B) {
	suffixes := []string{"b", "kb", "mb", "gb", "tb"}

	for _, suffix := range suffixes {
		b.Run(suffix, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = NewUnitFromSuffix(suffix)
			}
		})
	}
}
