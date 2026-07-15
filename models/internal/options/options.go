package options

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

func GetParams[T any](values metadata.Map, key string) (*T, error) {
	params, exists, err := metadata.Decode[T](values, key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return new(T), nil
	}
	return &params, nil
}

// RejectUnsupported returns a deterministic error naming every explicitly set
// common option that a provider cannot represent.
func RejectUnsupported(scope string, fields map[string]bool) error {
	unsupported := make([]string, 0, len(fields))
	for field, set := range fields {
		if set {
			unsupported = append(unsupported, field)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	slices.Sort(unsupported)
	return fmt.Errorf("%s: unsupported options: %s", scope, strings.Join(unsupported, ", "))
}

// Int converts a protocol int64 to a provider SDK int without platform-sized
// truncation.
func Int(scope string, value int64) (int, error) {
	converted := int(value)
	if int64(converted) != value {
		return 0, fmt.Errorf("%s: %d exceeds int", scope, value)
	}
	return converted, nil
}
