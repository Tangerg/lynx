package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RunQuery selects one cursor page in runtime order, newest first. An empty
// status set means all lifecycle states; descendants remain opt-in because
// their presence changes both topology and pagination.
type RunQuery struct {
	SessionID          string
	Statuses           []RunStatus
	IncludeDescendants bool
	Cursor             string
	Limit              int
}

func (query RunQuery) Validate() error {
	if query.Limit < 0 {
		return errors.New("run query: limit cannot be negative")
	}
	seen := make(map[RunStatus]struct{}, len(query.Statuses))
	for _, status := range query.Statuses {
		if !slices.Contains([]RunStatus{RunStatusRunning, RunStatusWaiting, RunStatusFinished}, status) {
			return fmt.Errorf("run query: status %q is invalid", status)
		}
		if _, duplicate := seen[status]; duplicate {
			return fmt.Errorf("run query: status %q is repeated", status)
		}
		seen[status] = struct{}{}
	}
	return nil
}

type RunPage struct {
	Items      []Run
	NextCursor string
}

func (page RunPage) Validate() error {
	seen := make(map[string]struct{}, len(page.Items))
	for index, run := range page.Items {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("run page item %d: %w", index+1, err)
		}
		if _, duplicate := seen[run.ID]; duplicate {
			return fmt.Errorf("run page repeats id %q", run.ID)
		}
		seen[run.ID] = struct{}{}
	}
	return nil
}

// RunCancellation is the atomic result of canceling one run-tree member.
// Canceled is always the addressed terminal run. Root is the authoritative
// root snapshot after that cancellation, which may still be active when only a
// child was canceled.
type RunCancellation struct {
	Canceled Run
	Root     Run
}

func (result RunCancellation) Validate() error {
	var problems []error
	if err := result.Canceled.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("canceled: %w", err))
	}
	if err := result.Root.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("root: %w", err))
	}
	if result.Canceled.Status != RunStatusFinished || result.Canceled.Outcome.Status != OutcomeCanceled {
		problems = append(problems, errors.New("addressed run is not finished as canceled"))
	}
	if !result.Root.Lineage.IsRoot() {
		problems = append(problems, errors.New("root projection is a child run"))
	}
	if result.Canceled.SessionID != result.Root.SessionID {
		problems = append(problems, errors.New("canceled run and root belong to different sessions"))
	}
	if result.Canceled.Lineage.IsRoot() {
		if !result.Canceled.Equal(result.Root) {
			problems = append(problems, errors.New("root cancellation carries two different root projections"))
		}
	} else if result.Canceled.Lineage.RootRunID != result.Root.ID {
		problems = append(problems, errors.New("canceled child does not belong to the returned root"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run cancellation: %w", err)
	}
	return nil
}

func (result RunCancellation) ValidateTarget(runID string) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return errors.New("run cancellation: target run id is empty")
	}
	if result.Canceled.ID != runID {
		return fmt.Errorf("run cancellation: returned run %q, want %q", result.Canceled.ID, runID)
	}
	return nil
}
