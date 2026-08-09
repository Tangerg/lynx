package runs

import (
	"errors"
)

func (c *Coordinator) requireStartDependencies() error {
	switch {
	case c.rootStarts == nil:
		return errors.New("runs: root execution starter is required")
	case c.conversation == nil:
		return errors.New("runs: conversation reader is required")
	case c.sessionCreator == nil:
		return errors.New("runs: session creator is required")
	case c.activeRuns == nil:
		return errors.New("runs: active run reader is required")
	case c.newRunID == nil:
		return errors.New("runs: run id generator is required")
	default:
		return c.requireSegmentDependencies()
	}
}

func (c *Coordinator) requireControlDependencies() error {
	if c.releases == nil {
		return errors.New("runs: execution releaser is required")
	}
	if c.sessionReader == nil {
		return errors.New("runs: session reader is required")
	}
	if c.interrupts == nil {
		return errors.New("runs: pending interrupt reader is required")
	}
	if c.terminations == nil {
		return errors.New("runs: run termination committer is required")
	}
	if c.admission == nil {
		return errors.New("runs: admission gate is required")
	}
	if c.runs == nil {
		return errors.New("runs: run projection is required")
	}
	return nil
}

func (c *Coordinator) requireResumeDependencies() error {
	switch {
	case c.continuation == nil:
		return errors.New("runs: waiting execution continuer is required")
	case c.interrupts == nil:
		return errors.New("runs: pending interrupt reader is required")
	case c.terminations == nil:
		return errors.New("runs: run termination committer is required")
	case c.resumeClaims == nil:
		return errors.New("runs: resume claim committer is required")
	case c.runs == nil:
		return errors.New("runs: run projection is required")
	default:
		return c.requireSegmentDependencies()
	}
}

// requireSegmentDependencies validates the segment-supervision collaborators
// shared by fresh Start and Resume. Each use case validates its own staging
// dependencies before delegating to this common lifecycle boundary.
func (c *Coordinator) requireSegmentDependencies() error {
	switch {
	case c.observations == nil:
		return errors.New("runs: execution observer is required")
	case c.releases == nil:
		return errors.New("runs: execution releaser is required")
	case c.sessionReader == nil:
		return errors.New("runs: session reader is required")
	case c.openings == nil:
		return errors.New("runs: opening committer is required")
	case c.events == nil:
		return errors.New("runs: event committer is required")
	case c.barriers == nil:
		return errors.New("runs: tree barrier committer is required")
	case c.workspace == nil:
		return errors.New("runs: workspace change notifier is required")
	case c.finalizer == nil:
		return errors.New("runs: segment finalizer is required")
	case c.admission == nil:
		return errors.New("runs: admission gate is required")
	case c.now == nil:
		return errors.New("runs: clock is required")
	case c.newSegmentID == nil:
		return errors.New("runs: segment id generator is required")
	default:
		return nil
	}
}
