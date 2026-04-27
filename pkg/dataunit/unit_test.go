package dataunit

import (
	"strings"
	"testing"
)

func TestSuffixConstants(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{BSuffix, "B"},
		{KBSuffix, "KB"},
		{MBSuffix, "MB"},
		{GBSuffix, "GB"},
		{TBSuffix, "TB"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestNewUnitFromSuffix(t *testing.T) {
	tests := []struct {
		suffix     string
		wantSize   DataSize
		wantSuffix string
	}{
		{"B", DataSize(B), BSuffix},
		{"KB", DataSize(KB), KBSuffix},
		{"MB", DataSize(MB), MBSuffix},
		{"GB", DataSize(GB), GBSuffix},
		{"TB", DataSize(TB), TBSuffix},
		{"b", DataSize(B), BSuffix},
		{"kb", DataSize(KB), KBSuffix},
		{"Mb", DataSize(MB), MBSuffix},
	}
	for _, tt := range tests {
		t.Run(tt.suffix, func(t *testing.T) {
			u, err := NewUnitFromSuffix(tt.suffix)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if u.Size() != tt.wantSize {
				t.Errorf("Size = %d, want %d", u.Size(), tt.wantSize)
			}
			if u.Suffix() != tt.wantSuffix {
				t.Errorf("Suffix = %q, want %q", u.Suffix(), tt.wantSuffix)
			}
		})
	}
}

func TestNewUnitFromSuffix_Invalid(t *testing.T) {
	tests := []string{"", "X", "PB", "kbz", "K B"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			u, err := NewUnitFromSuffix(in)
			if err == nil {
				t.Fatalf("expected error for %q", in)
			}
			if u != nil {
				t.Errorf("expected nil unit, got %v", u)
			}
			// Error message should mention the bad suffix.
			if !strings.Contains(err.Error(), "suffix") {
				t.Errorf("error %q lacks 'suffix'", err)
			}
		})
	}
}

func TestDataUnit_Accessors(t *testing.T) {
	u := &DataUnit{size: DataSize(2048), suffix: KBSuffix}
	if u.Size() != 2048 {
		t.Errorf("Size = %d, want 2048", u.Size())
	}
	if u.Suffix() != KBSuffix {
		t.Errorf("Suffix = %q, want %q", u.Suffix(), KBSuffix)
	}
}
