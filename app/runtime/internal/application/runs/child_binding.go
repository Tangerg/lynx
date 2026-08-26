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
func (c ChildRunBinding) Validate() error {
	switch {
	case c.MemberID == "":
		return errors.New("runs: child Run binding has no executor member id")
	case strings.TrimSpace(c.MemberID) != c.MemberID:
		return fmt.Errorf("runs: child Run binding member id %q has surrounding whitespace", c.MemberID)
	case c.RunID == "":
		return errors.New("runs: child Run binding has no Run id")
	case strings.TrimSpace(c.RunID) != c.RunID:
		return fmt.Errorf("runs: child Run binding Run id %q has surrounding whitespace", c.RunID)
	case c.ParentRunID == "":
		return errors.New("runs: child Run binding has no parent Run id")
	case strings.TrimSpace(c.ParentRunID) != c.ParentRunID:
		return fmt.Errorf("runs: child Run binding parent Run id %q has surrounding whitespace", c.ParentRunID)
	case c.RunID == c.ParentRunID:
		return errors.New("runs: child Run binding refers to itself as parent")
	default:
		return nil
	}
}
