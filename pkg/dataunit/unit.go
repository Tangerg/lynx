package dataunit

import (
	"fmt"
	"strings"
)

// Constants representing suffixes for different data units.
const (
	BSuffix  = "B"
	KBSuffix = "KB"
	MBSuffix = "MB"
	GBSuffix = "GB"
	TBSuffix = "TB"
)

// DataUnit represents a data size with its corresponding suffix.
type DataUnit struct {
	size   DataSize // The size of the data unit in bytes
	suffix string   // The suffix representing the data unit (e.g., "KB", "MB")
}

func (u *DataUnit) Size() DataSize {
	return u.size
}

func (u *DataUnit) Suffix() string {
	return u.suffix
}

// NewUnitFromSuffix creates a new DataUnit based on the provided suffix.
// Returns an error if the suffix is unknown.
func NewUnitFromSuffix(suffix string) (*DataUnit, error) {
	r := &DataUnit{}
	suffix = strings.ToUpper(suffix)
	if suffix == BSuffix {
		r.size = SizeOfB(1)
		r.suffix = BSuffix
		return r, nil
	}
	if suffix == KBSuffix {
		r.size, _ = SizeOfKB(1)
		r.suffix = KBSuffix
		return r, nil
	}
	if suffix == MBSuffix {
		r.size, _ = SizeOfMB(1)
		r.suffix = MBSuffix
		return r, nil
	}
	if suffix == GBSuffix {
		r.size, _ = SizeOfGB(1)
		r.suffix = GBSuffix
		return r, nil
	}
	if suffix == TBSuffix {
		r.size, _ = SizeOfTB(1)
		r.suffix = TBSuffix
		return r, nil
	}
	return nil, fmt.Errorf("unknown data unit suffix: %s", suffix)
}
