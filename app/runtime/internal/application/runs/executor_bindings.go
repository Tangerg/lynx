package runs

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

// bindExecutorProcess records the immutable application-Run to opaque executor
// identity owned by this root segment. Executor parent/spawn topology stays in
// executorRoutes; cancellation needs only the exact process identity it must
// address. A child binding is reserved before its durable opening commit,
// closing the otherwise observable gap in which the Run row exists but
// cancellation cannot address its process.
func (h *handle) bindExecutorProcess(runID, processID string) error {
	if h == nil {
		return errors.New("runs: bind executor process without a live root handle")
	}
	if strings.TrimSpace(runID) == "" || runID != strings.TrimSpace(runID) {
		return errors.New("runs: bind executor process without a canonical Run id")
	}
	if strings.TrimSpace(processID) == "" || processID != strings.TrimSpace(processID) {
		return fmt.Errorf("runs: bind Run %q without a canonical executor process id", runID)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorProcesses[runID]; bound {
		if existing != processID {
			return fmt.Errorf(
				"runs: Run %q executor process changed from %q to %q",
				runID,
				existing,
				processID,
			)
		}
		return nil
	}
	for existingRunID, existing := range h.executorProcesses {
		if existing == processID {
			return fmt.Errorf(
				"runs: executor process %q is already bound to Run %q, not %q",
				processID,
				existingRunID,
				runID,
			)
		}
	}
	if h.executorProcesses == nil {
		h.executorProcesses = make(map[string]string)
	}
	h.executorProcesses[runID] = processID
	return nil
}

// unbindExecutorProcess rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (h *handle) unbindExecutorProcess(runID, processID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorProcesses[runID]; bound && existing == processID {
		delete(h.executorProcesses, runID)
	}
}

func (h *handle) executorProcessSnapshot() map[string]string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	processes := make(map[string]string, len(h.executorProcesses))
	maps.Copy(processes, h.executorProcesses)
	return processes
}
