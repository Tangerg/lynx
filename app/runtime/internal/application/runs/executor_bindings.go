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
func (h *handle) bindExecutorMember(runID, memberID string) error {
	if h == nil {
		return errors.New("runs: bind executor member without a live root handle")
	}
	if strings.TrimSpace(runID) == "" || runID != strings.TrimSpace(runID) {
		return errors.New("runs: bind executor member without a canonical Run id")
	}
	if strings.TrimSpace(memberID) == "" || memberID != strings.TrimSpace(memberID) {
		return fmt.Errorf("runs: bind Run %q without a canonical executor member id", runID)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorMembers[runID]; bound {
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
	for existingRunID, existing := range h.executorMembers {
		if existing == memberID {
			return fmt.Errorf(
				"runs: executor member %q is already bound to Run %q, not %q",
				memberID,
				existingRunID,
				runID,
			)
		}
	}
	if h.executorMembers == nil {
		h.executorMembers = make(map[string]string)
	}
	h.executorMembers[runID] = memberID
	return nil
}

// unbindExecutorMember rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (h *handle) unbindExecutorMember(runID, memberID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorMembers[runID]; bound && existing == memberID {
		delete(h.executorMembers, runID)
	}
}

func (h *handle) executorMemberSnapshot() map[string]string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	members := make(map[string]string, len(h.executorMembers))
	maps.Copy(members, h.executorMembers)
	return members
}
