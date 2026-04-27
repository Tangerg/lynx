package dataunit

import (
	xmath "github.com/Tangerg/lynx/pkg/math"
)

// Byte multiplier constants using powers of 1024 (IEC units). Left
// untyped so they convert freely to int64 or [DataSize].
const (
	B  = 1
	KB = B * 1024
	MB = KB * 1024
	GB = MB * 1024
	TB = GB * 1024
)

// DataSize represents a quantity of bytes.
type DataSize int64

// Int64 returns the size in bytes as int64.
func (s DataSize) Int64() int64 { return int64(s) }

// Compare returns -1, 0, or 1 as s is less than, equal to, or greater
// than other.
func (s DataSize) Compare(other DataSize) int {
	switch {
	case s < other:
		return -1
	case s > other:
		return 1
	}
	return 0
}

// Negative reports whether s < 0.
func (s DataSize) Negative() bool { return s < 0 }

// Positive reports whether s > 0.
func (s DataSize) Positive() bool { return s > 0 }

// B returns the size in bytes.
func (s DataSize) B() int64 { return int64(s) }

// KB returns the size in kibibytes, rounded toward zero.
func (s DataSize) KB() int64 { return int64(s) / KB }

// MB returns the size in mebibytes, rounded toward zero.
func (s DataSize) MB() int64 { return int64(s) / MB }

// GB returns the size in gibibytes, rounded toward zero.
func (s DataSize) GB() int64 { return int64(s) / GB }

// TB returns the size in tebibytes, rounded toward zero.
func (s DataSize) TB() int64 { return int64(s) / TB }

// SizeOfB returns a DataSize equal to b bytes.
func SizeOfB(b int64) DataSize { return DataSize(b) }

// SizeOfKB returns kb*1024 bytes, or an error if the result overflows
// int64.
func SizeOfKB(kb int64) (DataSize, error) { return mulSize(kb, KB) }

// SizeOfMB returns mb*1024² bytes, or an error if the result overflows
// int64.
func SizeOfMB(mb int64) (DataSize, error) { return mulSize(mb, MB) }

// SizeOfGB returns gb*1024³ bytes, or an error if the result overflows
// int64.
func SizeOfGB(gb int64) (DataSize, error) { return mulSize(gb, GB) }

// SizeOfTB returns tb*1024⁴ bytes, or an error if the result overflows
// int64.
func SizeOfTB(tb int64) (DataSize, error) { return mulSize(tb, TB) }

// mulSize multiplies n by unit with overflow checking.
func mulSize(n, unit int64) (DataSize, error) {
	r, err := xmath.MultiplyExact(n, unit)
	if err != nil {
		return 0, err
	}
	return DataSize(r), nil
}
