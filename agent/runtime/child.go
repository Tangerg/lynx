package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// maxChildDepth limits recursive delegation. A root process has depth zero.
const maxChildDepth = 8

// ErrChildDepth reports that a child would exceed the delegation depth limit.
var ErrChildDepth = errors.New("run child: max delegation depth exceeded")

// StartChild starts a child in the background and returns immediately. Like
// [Engine.RunChild], it copies only the parent's protected ambient state. The
// returned channel receives the terminal run error, if any.
//
// The child outlives cancellation of the calling action. Callers own its
// lifecycle through [Engine.Process], [Engine.Kill], or [Engine.KillChildren].
func (e *Engine) StartChild(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, <-chan error, error) {
	if e == nil {
		return nil, nil, errors.New("start child: engine is nil")
	}
	deployment, err := e.ownedDeployment("start child", deployment)
	if err != nil {
		return nil, nil, err
	}
	return startChildDeployment(ctx, e, deployment, input)
}

func startChildDeployment(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, <-chan error, error) {
	run := childRun{
		ctx:        ctx,
		engine:     engine,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}
	child, err := run.create()
	if err != nil {
		return nil, nil, err
	}
	done := engine.ContinueAsync(context.WithoutCancel(ctx), child.ID())
	return child, done, nil
}

// RunChildWithState runs a child with a copy of the parent's entire blackboard.
// Use it only when the child needs the parent's working state. For ordinary
// delegation, prefer [Engine.RunChild], which copies ambient state only.
func (e *Engine) RunChildWithState(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childCopiesParentState,
	}.run()
}

// RunChild runs a child with a clean blackboard containing only the parent's
// protected ambient state. This is the safe default for self-contained
// delegation. The parent process must be attached to ctx with
// [core.WithProcessView].
func (e *Engine) RunChild(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}.run()
}

func runChildDeployment(
	ctx context.Context,
	engine *Engine,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     engine,
		deployment: deployment,
		input:      input,
		mode:       childCopiesAmbientState,
	}.run()
}

// RunChildIsolated runs a child with a fresh blackboard seeded only with input.
// Use it for loops, pipelines, and parallel branches that must not inherit even
// ambient state.
func (e *Engine) RunChildIsolated(
	ctx context.Context,
	deployment *Deployment,
	input any,
) (*Process, error) {
	return childRun{
		ctx:        ctx,
		engine:     e,
		deployment: deployment,
		input:      input,
		mode:       childStartsEmpty,
	}.run()
}

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
		return child, fmt.Errorf("run child %q (process %q): run: %w", child.agent().Name(), child.ID(), err)
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

	options, err := r.processOptions(parent, deployment)
	if err != nil {
		return nil, fmt.Errorf("run child %q: options: %w", agentName, err)
	}
	child, err := r.engine.createChild(deployment, parent, r.bindings(), options)
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

func (r childRun) bindings() core.Bindings {
	if r.input == nil {
		return core.Bindings{}
	}
	return core.Input(r.input)
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

func (r childRun) processOptions(parent *Process, deployment *Deployment) (core.ProcessOptions, error) {
	var options core.ProcessOptions
	switch r.mode {
	case childCopiesAmbientState:
		options.Blackboard = ambientBlackboard(parent.blackboard)
	case childStartsEmpty:
		options.Blackboard = r.engine.NewBlackboard()
	}
	return configureChildProcessOptions(r.ctx, parent, deployment, options)
}

func configureChildProcessOptions(
	ctx context.Context,
	parent *Process,
	deployment *Deployment,
	options core.ProcessOptions,
) (core.ProcessOptions, error) {
	if parent == nil || parent.options == nil || parent.options.childOptions == nil {
		return options, nil
	}
	configure := parent.options.childOptions
	configured, err := configure(normalizeContext(ctx), parent, deployment.agent)
	if err != nil {
		return core.ProcessOptions{}, err
	}
	if configured.Blackboard == nil {
		configured.Blackboard = options.Blackboard
	}
	if configured.ChildOptions == nil {
		configured.ChildOptions = configure
	}
	return configured, nil
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
	if child.options == nil || child.options.session != nil {
		return nil
	}
	parentConvID := parent.conversationID()
	if parentConvID == "" {
		return nil
	}
	session := core.NewSession(child.ID(), parent.userID(), child.agent().Name())
	session.ParentID = parentConvID
	child.options.session = &session

	if r.engine.childSessionStore != nil {
		if err := r.engine.childSessionStore.Save(r.ctx, session); err != nil {
			return fmt.Errorf("save child session %q: %w", child.ID(), err)
		}
	}
	return nil
}

func (r childRun) restoreSession(child, parent *Process) error {
	if child == nil || parent == nil || child.options == nil || child.options.session != nil {
		return nil
	}
	r.ctx = normalizeContext(r.ctx)
	parentConversationID := parent.conversationID()
	if parentConversationID == "" {
		return nil
	}
	if r.engine == nil || r.engine.childSessionStore == nil {
		return r.linkSession(child, parent)
	}
	session, err := r.engine.childSessionStore.Load(r.ctx, child.ID())
	if err != nil {
		if errors.Is(err, core.ErrSessionNotFound) {
			return r.linkSession(child, parent)
		}
		return fmt.Errorf("load child session %q: %w", child.ID(), err)
	}
	if err := session.Validate(); err != nil {
		return fmt.Errorf("stored child session %q: %w", child.ID(), err)
	}
	if session.ID != child.ID() ||
		session.ParentID != parentConversationID ||
		session.UserID != parent.userID() ||
		session.AgentName != child.agent().Name() {
		return fmt.Errorf(
			"stored child session %q identity is parent=%q user=%q agent=%q; want parent=%q user=%q agent=%q",
			session.ID,
			session.ParentID,
			session.UserID,
			session.AgentName,
			parentConversationID,
			parent.userID(),
			child.agent().Name(),
		)
	}
	child.options.session = &session
	return nil
}
