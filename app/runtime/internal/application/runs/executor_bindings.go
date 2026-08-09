package runs

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

// bindExecutorMember records the immutable application-Run to opaque executor
// identity owned by this root segment. Executor parent/spawn topology stays in
// executorRoutes; cancellation needs only the exact member identity it must
// address. A child binding is reserved before its durable opening commit,
// closing the otherwise observable gap in which the Run row exists but
// cancellation cannot address its member.
func (owner *runTreeOwner) bindExecutorMember(runID, memberID string) error {
	if owner == nil {
		return errors.New("runs: bind executor member without a live Run-tree owner")
	}
	if strings.TrimSpace(runID) == "" || runID != strings.TrimSpace(runID) {
		return errors.New("runs: bind executor member without a canonical Run id")
	}
	if strings.TrimSpace(memberID) == "" || memberID != strings.TrimSpace(memberID) {
		return fmt.Errorf("runs: bind Run %q without a canonical executor member id", runID)
	}

	owner.mu.Lock()
	defer owner.mu.Unlock()
	if existing, bound := owner.executorMembers[runID]; bound {
		if existing != memberID {
			return fmt.Errorf(
				"runs: Run %q executor member changed from %q to %q",
				runID,
				existing,
				memberID,
			)
		}
		return nil
	}
	for existingRunID, existing := range owner.executorMembers {
		if existing == memberID {
			return fmt.Errorf(
				"runs: executor member %q is already bound to Run %q, not %q",
				memberID,
				existingRunID,
				runID,
			)
		}
	}
	if owner.executorMembers == nil {
		owner.executorMembers = make(map[string]string)
	}
	owner.executorMembers[runID] = memberID
	return nil
}

// unbindExecutorMember rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (owner *runTreeOwner) unbindExecutorMember(runID, memberID string) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if existing, bound := owner.executorMembers[runID]; bound && existing == memberID {
		delete(owner.executorMembers, runID)
	}
}

func (owner *runTreeOwner) executorMemberSnapshot() map[string]string {
	if owner == nil {
		return nil
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	members := make(map[string]string, len(owner.executorMembers))
	maps.Copy(members, owner.executorMembers)
	return members
}
