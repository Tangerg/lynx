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
func (r *runTreeOwner) bindExecutorMember(runID, memberID string) error {
	if r == nil {
		return errors.New("runs: bind executor member without a live Run-tree owner")
	}
	if strings.TrimSpace(runID) == "" || runID != strings.TrimSpace(runID) {
		return errors.New("runs: bind executor member without a canonical Run id")
	}
	if strings.TrimSpace(memberID) == "" || memberID != strings.TrimSpace(memberID) {
		return fmt.Errorf("runs: bind Run %q without a canonical executor member id", runID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, bound := r.executorMembers[runID]; bound {
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
	for existingRunID, existing := range r.executorMembers {
		if existing == memberID {
			return fmt.Errorf(
				"runs: executor member %q is already bound to Run %q, not %q",
				memberID,
				existingRunID,
				runID,
			)
		}
	}
	if r.executorMembers == nil {
		r.executorMembers = make(map[string]string)
	}
	r.executorMembers[runID] = memberID
	return nil
}

// unbindExecutorMember rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (r *runTreeOwner) unbindExecutorMember(runID, memberID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, bound := r.executorMembers[runID]; bound && existing == memberID {
		delete(r.executorMembers, runID)
	}
}

func (r *runTreeOwner) executorMemberSnapshot() map[string]string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	members := make(map[string]string, len(r.executorMembers))
	maps.Copy(members, r.executorMembers)
	return members
}
