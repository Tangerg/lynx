package runs

import (
	"errors"
)

func (c *Coordinator) requireUseCaseDependencies() error {
	switch {
	case c.executor == nil:
		return errors.New("runs: segment executor is required")
	case c.turns == nil:
		return errors.New("runs: turn control is required")
	case c.sessions == nil:
		return errors.New("runs: session lifecycle is required")
	case c.effects == nil:
		return errors.New("runs: effects are required")
	case c.admission == nil:
		return errors.New("runs: admission gate is required")
	case c.now == nil:
		return errors.New("runs: clock is required")
	case c.newRunID == nil:
		return errors.New("runs: run id generator is required")
	case c.newSegmentID == nil:
		return errors.New("runs: segment id generator is required")
	default:
		return nil
	}
}

func (c *Coordinator) requireControlDependencies() error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	if c.sessions == nil {
		return errors.New("runs: session lifecycle is required")
	}
	if c.admission == nil {
		return errors.New("runs: admission gate is required")
	}
	if c.runs == nil {
		return errors.New("runs: run projection is required")
	}
	return nil
}
