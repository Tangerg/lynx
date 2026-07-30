package runs

import (
	"errors"
	"fmt"
)

// bindExecutorSource records the immutable application-Run to executor-process
// edge owned by this root segment. A child binding is reserved before its
// durable opening commit, closing the otherwise observable gap in which the Run
// row exists but cancellation cannot address its process.
func (h *handle) bindExecutorSource(runID string, source ExecutorSource) error {
	if h == nil {
		return errors.New("runs: bind executor source without a live root handle")
	}
	if runID == "" {
		return errors.New("runs: bind executor source without a run id")
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("runs: bind Run %q executor source: %w", runID, err)
	}
	if source.ProcessID == "" {
		return fmt.Errorf("runs: bind Run %q executor source without a process id", runID)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorSources[runID]; bound {
		if existing != source {
			return fmt.Errorf(
				"runs: Run %q executor source changed from %q to %q",
				runID,
				existing.ProcessID,
				source.ProcessID,
			)
		}
		return nil
	}
	for existingRunID, existing := range h.executorSources {
		if existing.ProcessID == source.ProcessID {
			return fmt.Errorf(
				"runs: executor process %q is already bound to Run %q, not %q",
				source.ProcessID,
				existingRunID,
				runID,
			)
		}
	}
	if h.executorSources == nil {
		h.executorSources = make(map[string]ExecutorSource)
	}
	h.executorSources[runID] = source
	return nil
}

// unbindExecutorSource rolls back a pre-commit child reservation. It removes
// only the exact binding this opening installed, so it cannot erase a later
// owner after a conflict.
func (h *handle) unbindExecutorSource(runID string, source ExecutorSource) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, bound := h.executorSources[runID]; bound && existing == source {
		delete(h.executorSources, runID)
	}
}

func (h *handle) executorSourceSnapshot() map[string]ExecutorSource {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	sources := make(map[string]ExecutorSource, len(h.executorSources))
	for runID, source := range h.executorSources {
		sources[runID] = source
	}
	return sources
}
