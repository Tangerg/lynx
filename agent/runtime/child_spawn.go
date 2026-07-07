package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

type childBlackboardInheritance int

const (
	childInheritsAll childBlackboardInheritance = iota
	childInheritsProtectedOnly
	childInheritsNothing
)

type childSpawn struct {
	ctx         context.Context
	platform    *Platform
	agentDef    *core.Agent
	input       any
	inheritance childBlackboardInheritance
}

func (s childSpawn) run() (*AgentProcess, error) {
	child, err := s.prepare()
	if err != nil {
		return nil, err
	}
	if err := s.platform.ContinueProcess(s.ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("spawn child %q (process %q): run: %w", s.agentDef.Name, child.ID(), err)
	}
	return child, nil
}

func (s childSpawn) prepare() (*AgentProcess, error) {
	parentProc, err := s.parent()
	if err != nil {
		return nil, err
	}
	// Structural backstop: refuse before creating the child, so a runaway
	// delegation chain fails fast instead of burning budget spawning deeper.
	if parentProc.depth+1 > maxSpawnDepth {
		return nil, fmt.Errorf("spawn child %q: %w (depth %d > max %d)", s.agentDef.Name, ErrMaxSpawnDepth, parentProc.depth+1, maxSpawnDepth)
	}

	child, err := s.platform.CreateChildProcess(s.agentDef, parentProc, s.options(parentProc))
	if err != nil {
		return nil, fmt.Errorf("spawn child %q: create: %w", s.agentDef.Name, err)
	}
	if err := s.linkSession(child, parentProc); err != nil {
		// CreateChildProcess registered the child AND joined it to the parent's
		// budget tree; linking its session failed, so undo BOTH — unregister it
		// from the platform and drop it from the parent's budget rollup. Either
		// left behind leaks: a never-started child sits at StatusNotStarted
		// (which PruneTerminalProcesses skips), and a stale budget child ref
		// lingers for the parent's whole life.
		_ = s.platform.RemoveProcess(child.ID())
		parentProc.budget.removeChild(child)
		return nil, fmt.Errorf("spawn child %q: link session: %w", s.agentDef.Name, err)
	}
	if s.input != nil {
		child.Blackboard().Bind(s.input)
	}
	return child, nil
}

func (s childSpawn) parent() (*AgentProcess, error) {
	if s.platform == nil {
		return nil, errors.New("spawn child: platform is nil")
	}
	if s.agentDef == nil {
		return nil, errors.New("spawn child: agent is nil")
	}
	parent := core.ProcessFrom(s.ctx)
	if parent == nil {
		return nil, errors.New("spawn child: no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := s.platform.ProcessByID(parent.ID())
	if !ok {
		return nil, fmt.Errorf("spawn child: parent process %q not registered on platform", parent.ID())
	}
	return parentProc, nil
}

func (s childSpawn) options(parent *AgentProcess) core.ProcessOptions {
	switch s.inheritance {
	case childInheritsProtectedOnly:
		return core.ProcessOptions{Blackboard: protectedOnlyBlackboard(parent.Blackboard())}
	case childInheritsNothing:
		return core.ProcessOptions{Blackboard: s.platform.NewBlackboard()}
	default:
		return core.ProcessOptions{}
	}
}

// protectedOnlyBlackboard returns a child blackboard that keeps only the
// parent's protected entries: [core.Blackboard.Spawn] copies all state, then
// [core.Blackboard.Clear] drops everything except entries bound via
// BindProtected. The result is a clean working surface that still carries the
// parent's ambient / session context.
func protectedOnlyBlackboard(parent core.Blackboard) core.Blackboard {
	bb := parent.Spawn()
	bb.Clear()
	return bb
}

// linkSession gives the child its own conversation while preserving delegation
// lineage through ParentID. Explicitly pinned sessions are left untouched.
func (s childSpawn) linkSession(child, parent *AgentProcess) error {
	if child.options == nil || child.options.Session != nil {
		return nil
	}
	parentConvID := parent.conversationID()
	if parentConvID == "" {
		return nil
	}
	session := core.NewSession(child.ID(), parent.userID(), s.agentDef.Name)
	session.ParentID = parentConvID
	child.options.Session = &session

	if s.platform.sessionStore != nil {
		if err := s.platform.sessionStore.Save(s.ctx, session); err != nil {
			return err
		}
	}
	return nil
}
