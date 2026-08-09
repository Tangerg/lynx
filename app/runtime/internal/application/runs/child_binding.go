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

// ValidateChildRunBindings proves that every restored child belongs to one
// connected application Run tree rooted at rootRunID. Executor topology is
// deliberately absent from this validation.
func ValidateChildRunBindings(rootRunID string, bindings []ChildRunBinding) error {
	if rootRunID == "" || strings.TrimSpace(rootRunID) != rootRunID {
		return errors.New("runs: restored child Run bindings require a root Run id")
	}
	byRun := make(map[string]ChildRunBinding, len(bindings))
	byMember := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.RunID == rootRunID {
			return fmt.Errorf("runs: root Run %q cannot appear as a child binding", rootRunID)
		}
		if _, exists := byRun[binding.RunID]; exists {
			return fmt.Errorf("runs: duplicate child Run binding %q", binding.RunID)
		}
		if runID, exists := byMember[binding.MemberID]; exists {
			return fmt.Errorf(
				"runs: executor member %q is bound to child Runs %q and %q",
				binding.MemberID,
				runID,
				binding.RunID,
			)
		}
		byRun[binding.RunID] = binding
		byMember[binding.MemberID] = binding.RunID
	}
	for _, binding := range bindings {
		seen := map[string]struct{}{binding.RunID: {}}
		parentRunID := binding.ParentRunID
		for parentRunID != rootRunID {
			parent, exists := byRun[parentRunID]
			if !exists {
				return fmt.Errorf(
					"runs: child Run %q refers to unknown parent Run %q",
					binding.RunID,
					parentRunID,
				)
			}
			if _, cyclic := seen[parentRunID]; cyclic {
				return fmt.Errorf("runs: child Run binding cycle reaches %q", parentRunID)
			}
			seen[parentRunID] = struct{}{}
			parentRunID = parent.ParentRunID
		}
	}
	return nil
}
