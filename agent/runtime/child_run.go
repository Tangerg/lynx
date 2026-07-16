package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

type childBlackboardMode int

const (
	childCopiesParentState childBlackboardMode = iota
	childCopiesAmbientState
	childStartsEmpty
)

type childRun struct {
	ctx        context.Context
	engine     *Engine
	deployment *Deployment
	input      any
	mode       childBlackboardMode
}

func (r childRun) run() (*Process, error) {
	child, err := r.create()
	if err != nil {
		return nil, err
	}
	if err := r.engine.Continue(r.ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("run child %q (process %q): run: %w", child.agent().Name(), child.ID(), err)
	}
	return child, nil
}

func (r childRun) create() (*Process, error) {
	parent, err := r.parentProcess()
	if err != nil {
		return nil, err
	}
	deployment, err := r.engine.ownedDeployment("run child", r.deployment)
	if err != nil {
		return nil, err
	}
	agentName := deployment.agent.Name()
	if parent.depth+1 > maxChildDepth {
		return nil, fmt.Errorf("run child %q: %w (depth %d > max %d)", agentName, ErrChildDepth, parent.depth+1, maxChildDepth)
	}

	child, err := r.engine.createChild(deployment, parent, r.bindings(), r.processOptions(parent))
	if err != nil {
		return nil, fmt.Errorf("run child %q: create: %w", agentName, err)
	}
	if err := r.linkSession(child, parent); err != nil {
		_ = r.engine.Remove(child.ID())
		parent.budget.removeChild(child)
		return nil, fmt.Errorf("run child %q: link session: %w", agentName, err)
	}
	return child, nil
}

func (r childRun) bindings() map[string]any {
	if r.input == nil {
		return nil
	}
	return map[string]any{core.DefaultBindingName: r.input}
}

func (r childRun) parentProcess() (*Process, error) {
	if r.engine == nil {
		return nil, errors.New("run child: engine is nil")
	}
	if r.deployment == nil {
		return nil, errors.New("run child: deployment is nil")
	}
	parent := core.ProcessViewFrom(r.ctx)
	if parent == nil {
		return nil, errors.New("run child: no parent process in ctx (use core.WithProcessView to inject one)")
	}
	parentProcess, ok := r.engine.Process(parent.ID())
	if !ok {
		return nil, fmt.Errorf("run child: parent process %q not registered on engine", parent.ID())
	}
	return parentProcess, nil
}

func (r childRun) processOptions(parent *Process) core.ProcessOptions {
	switch r.mode {
	case childCopiesAmbientState:
		return core.ProcessOptions{Blackboard: ambientBlackboard(parent.blackboard)}
	case childStartsEmpty:
		return core.ProcessOptions{Blackboard: r.engine.NewBlackboard()}
	default:
		return core.ProcessOptions{}
	}
}

// ambientBlackboard copies the parent's protected entries into a clean board.
func ambientBlackboard(parent core.Blackboard) core.Blackboard {
	blackboard := parent.Clone()
	blackboard.ClearWorkingState()
	return blackboard
}

// linkSession gives the child its own conversation while preserving delegation
// lineage through ParentID. Explicitly pinned sessions are left untouched.
func (r childRun) linkSession(child, parent *Process) error {
	if child.options == nil || child.options.Session != nil {
		return nil
	}
	parentConvID := parent.conversationID()
	if parentConvID == "" {
		return nil
	}
	session := core.NewSession(child.ID(), parent.userID(), child.agent().Name())
	session.ParentID = parentConvID
	child.options.Session = &session

	if r.engine.sessionStore != nil {
		if err := r.engine.sessionStore.Save(r.ctx, session); err != nil {
			return err
		}
	}
	return nil
}
