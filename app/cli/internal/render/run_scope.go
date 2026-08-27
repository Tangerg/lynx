package render

import (
	"fmt"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

// runScope protects a renderer from mixing unrelated streams while allowing a
// root stream to carry events from its negotiated child-run tree.
type runScope struct {
	rootID  string
	members map[string]agent.RunLineage
}

func (r *runScope) bind(run agent.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if !run.Lineage.IsRoot() {
		return fmt.Errorf("run %s is not a root", run.ID)
	}
	if r.rootID != "" && r.rootID != run.ID {
		return fmt.Errorf("run %s does not match %s", run.ID, r.rootID)
	}
	r.ensureMembers()
	r.rootID = run.ID
	r.members[run.ID] = run.Lineage
	return nil
}

func (r *runScope) accept(envelope agent.RunEvent) error {
	started, opening := envelope.Event.(agent.SegmentStarted)
	if opening {
		run := started.Run
		if run.ID != envelope.RunID {
			return fmt.Errorf("segment start run %s does not match envelope %s", run.ID, envelope.RunID)
		}
		if run.Lineage.IsRoot() {
			return r.bind(run)
		}
		if r.rootID == "" || run.Lineage.RootRunID != r.rootID {
			return fmt.Errorf("child run %s does not belong to root %s", run.ID, r.rootID)
		}
		r.ensureMembers()
		if _, exists := r.members[run.Lineage.ParentRunID]; !exists {
			return fmt.Errorf("child run %s has unknown parent %s", run.ID, run.Lineage.ParentRunID)
		}
		if lineage, exists := r.members[run.ID]; exists && lineage != run.Lineage {
			return fmt.Errorf("child run %s changed lineage", run.ID)
		}
		r.members[run.ID] = run.Lineage
		return nil
	}
	if _, exists := r.members[envelope.RunID]; !exists {
		return fmt.Errorf("event references unknown run %s", envelope.RunID)
	}
	switch event := envelope.Event.(type) {
	case agent.BlockStarted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case agent.BlockCompleted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case agent.RunInterrupted:
		for _, interaction := range event.Interactions {
			if agent.InteractionRunID(interaction) != envelope.RunID {
				return fmt.Errorf("interrupt for run %s carries an interaction from run %s", envelope.RunID, agent.InteractionRunID(interaction))
			}
		}
	}
	return nil
}

func (r *runScope) restore(snapshot agent.SessionSnapshot, rootID string) error {
	run, exists := snapshot.RunByID(rootID)
	if !exists {
		return fmt.Errorf("run %s is absent from the snapshot", rootID)
	}
	if err := r.bind(run); err != nil {
		return err
	}
	for _, member := range snapshot.Runs {
		if member.Lineage.RootRunID == rootID {
			r.members[member.ID] = member.Lineage
		}
	}
	return nil
}

func (r *runScope) contains(runID string) bool {
	_, exists := r.members[runID]
	return exists
}

func (r *runScope) isRoot(runID string) bool { return runID != "" && runID == r.rootID }

func (r *runScope) isChild(runID string) bool {
	lineage, exists := r.members[runID]
	return exists && !lineage.IsRoot()
}

func (r *runScope) ensureMembers() {
	if r.members == nil {
		r.members = make(map[string]agent.RunLineage)
	}
}
