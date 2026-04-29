package dataunit

import (
	"fmt"
	"strings"
)

// Suffix constants used by [NewUnitFromSuffix].
const (
	BSuffix  = "B"
	KBSuffix = "KB"
	MBSuffix = "MB"
	GBSuffix = "GB"
	TBSuffix = "TB"
)

// DataUnit pairs a single-unit [DataSize] with its textual suffix.
// It is produced by [NewUnitFromSuffix] and used to format or parse
// human-readable byte sizes.
type DataUnit struct {
	size   DataSize
	suffix string
}

// Size returns the byte size of one of the unit (e.g. 1024 for KB).
func (u *DataUnit) Size() DataSize { return u.size }

// Suffix returns the textual suffix (e.g. "KB").
func (u *DataUnit) Suffix() string { return u.suffix }

// NewUnitFromSuffix returns the DataUnit corresponding to suffix.
// Matching is case-insensitive. It returns an error for unknown
// suffixes.
//
// Example:
//
//	u, _ := dataunit.NewUnitFromSuffix("MB")
//	bytesPerMB := u.Size() // 1048576
func NewUnitFromSuffix(suffix string) (*DataUnit, error) {
	switch strings.ToUpper(suffix) {
	case BSuffix:
		return &DataUnit{size: SizeOfB(1), suffix: BSuffix}, nil
	case KBSuffix:
		s, _ := SizeOfKB(1)
		return &DataUnit{size: s, suffix: KBSuffix}, nil
	case MBSuffix:
		s, _ := SizeOfMB(1)
		return &DataUnit{size: s, suffix: MBSuffix}, nil
	case GBSuffix:
		s, _ := SizeOfGB(1)
		return &DataUnit{size: s, suffix: GBSuffix}, nil
	case TBSuffix:
		s, _ := SizeOfTB(1)
		return &DataUnit{size: s, suffix: TBSuffix}, nil
	default:
		return nil, fmt.Errorf("dataunit: unknown suffix %q", suffix)
	}
}
