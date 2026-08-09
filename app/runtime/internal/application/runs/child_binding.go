package runs

import (
	"errors"
	"fmt"
	"strings"
)

// ChildRunBinding is the application identity assigned to one opaque executor
// child. Product lifecycle observers use Run identity; executor member identity
// remains an implementation detail.
type ChildRunBinding struct {
	MemberID    string
	RunID       string
	ParentRunID string
}

// Validate rejects incomplete or ambiguous child identity before it reaches a
// lifecycle observer.
func (binding ChildRunBinding) Validate() error {
	switch {
	case binding.MemberID == "":
		return errors.New("runs: child Run binding has no executor member id")
	case strings.TrimSpace(binding.MemberID) != binding.MemberID:
		return fmt.Errorf("runs: child Run binding member id %q has surrounding whitespace", binding.MemberID)
	case binding.RunID == "":
		return errors.New("runs: child Run binding has no Run id")
	case strings.TrimSpace(binding.RunID) != binding.RunID:
		return fmt.Errorf("runs: child Run binding Run id %q has surrounding whitespace", binding.RunID)
	case binding.ParentRunID == "":
		return errors.New("runs: child Run binding has no parent Run id")
	case strings.TrimSpace(binding.ParentRunID) != binding.ParentRunID:
		return fmt.Errorf("runs: child Run binding parent Run id %q has surrounding whitespace", binding.ParentRunID)
	case binding.RunID == binding.ParentRunID:
		return errors.New("runs: child Run binding refers to itself as parent")
	default:
		return nil
	}
}
