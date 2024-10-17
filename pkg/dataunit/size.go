package dataunit

import (
	xmath "github.com/Tangerg/lynx/pkg/math"
)

// Constants representing byte sizes for different data units.
const (
	B  = 1
	KB = B * 1024
	MB = KB * 1024
	GB = MB * 1024
	TB = GB * 1024
)

// DataSize is a custom type for representing data sizes in bytes.
type DataSize int64

func (s DataSize) Int64() int64 {
	return int64(s)
}

// Compare compares the current DataSize with another.
// Returns 0 if equal, -1 if less, and 1 if greater.
func (s DataSize) Compare(other DataSize) int {
	if s == other {
		return 0
	}
	if s < other {
		return -1
	}
	return 1
}

// Negative checks if the DataSize is negative.
func (s DataSize) Negative() bool {
	return s < 0
}

// Positive checks if the DataSize is positive.
func (s DataSize) Positive() bool {
	return s > 0
}

// B returns the size in bytes.
func (s DataSize) B() int64 {
	return int64(s)
}

// KB returns the size in kilobytes.
func (s DataSize) KB() int64 {
	return int64(s) / KB
}

// MB returns the size in megabytes.
func (s DataSize) MB() int64 {
	return int64(s) / MB
}

// GB returns the size in gigabytes.
func (s DataSize) GB() int64 {
	return int64(s) / GB
}

// TB returns the size in terabytes.
func (s DataSize) TB() int64 {
	return int64(s) / TB
}

// SizeOfB creates a DataSize from a byte value.
func SizeOfB(b int64) DataSize {
	return DataSize(b)
}

// SizeOfKB creates a DataSize from a kilobyte value, checking for overflow.
func SizeOfKB(kb int64) (DataSize, error) {
	r, err := xmath.MultiplyExact(kb, KB)
	if err != nil {
		return DataSize(0), err
	}
	return DataSize(r), nil
}

// SizeOfMB creates a DataSize from a megabyte value, checking for overflow.
func SizeOfMB(mb int64) (DataSize, error) {
	r, err := xmath.MultiplyExact(mb, MB)
	if err != nil {
		return DataSize(0), err
	}
	return DataSize(r), nil
}

// SizeOfGB creates a DataSize from a gigabyte value, checking for overflow.
func SizeOfGB(gb int64) (DataSize, error) {
	r, err := xmath.MultiplyExact(gb, GB)
	if err != nil {
		return DataSize(0), err
	}
	return DataSize(r), nil
}

// SizeOfTB creates a DataSize from a terabyte value, checking for overflow.
func SizeOfTB(tb int64) (DataSize, error) {
	r, err := xmath.MultiplyExact(tb, TB)
	if err != nil {
		return DataSize(0), err
	}
	return DataSize(r), nil
}
