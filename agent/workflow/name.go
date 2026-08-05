package workflow

import (
	"fmt"
	"strings"
)

func validateName(operation, name string) error {
	switch {
	case name == "":
		return fmt.Errorf("workflow.%s: Name must not be empty", operation)
	case strings.TrimSpace(name) != name:
		return fmt.Errorf("workflow.%s: Name must not have surrounding whitespace", operation)
	default:
		return nil
	}
}
