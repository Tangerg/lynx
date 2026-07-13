package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ReadSnapshot reserves the session's single-writer slot and reads its complete
// canonical state coherently. Active and parked runs are rejected because their
// executor state is process-local and therefore cannot be represented by a
// portable session artifact.
func (c *Coordinator) ReadSnapshot(ctx context.Context, claims SessionClaimer, sessionID string) (Snapshot, error) {
	admission, err := c.ClaimRunSlot(ctx, claims, sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	defer admission.Release()
	snapshot, err := c.s.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func validateSnapshot(snapshot Snapshot) error {
	runs := make(map[string]struct{}, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		if !run.State.IsTerminal() {
			return fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID, run.State)
		}
		if run.Outcome == nil || run.Result == nil {
			return fmt.Errorf("sessions: snapshot terminal run %q has no outcome or result", run.ID)
		}
		if run.MessageMark < 0 || run.MessageMark > len(snapshot.Messages) {
			return fmt.Errorf("sessions: snapshot run %q has invalid message watermark %d", run.ID, run.MessageMark)
		}
		runs[run.ID] = struct{}{}
	}
	for _, item := range snapshot.Items {
		if _, found := runs[item.RunID]; !found {
			return fmt.Errorf("sessions: snapshot item %q references unknown run %q", item.ID, item.RunID)
		}
		if item.Status == transcript.ItemRunning {
			return fmt.Errorf("sessions: snapshot terminal run item %q is still running", item.ID)
		}
	}
	return nil
}
