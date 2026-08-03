package openai

import (
	"fmt"
	"slices"
	"strings"
)

func rejectUnsupportedOptions(scope string, fields map[string]bool) error {
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
