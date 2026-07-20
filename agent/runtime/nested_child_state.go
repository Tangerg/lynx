package runtime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
)

// nestedChildState owns child relations produced by concurrently executing
// AgentTools. staged relations have not reached a parent checkpoint; pending
// relations have. Engine side effects run only after IDs leave this lock so
// child lifecycle code never introduces a child-to-parent lock cycle.
type nestedChildState struct {
	mu      sync.Mutex
	staged  map[string]*nestedChildRelation
	pending map[string]*nestedChildRelation
	cleanup []string
}

func (s *nestedChildState) stage(processID string, relation *nestedChildRelation) error {
	if err := relation.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if current := s.staged[relation.ToolCallID]; current != nil {
		if current.same(relation) {
			return nil
		}
		return fmt.Errorf("%w: process %q already staged tool call %q", interaction.ErrSuspensionConflict, processID, relation.ToolCallID)
	}
	if current := s.pending[relation.ToolCallID]; current != nil && !current.sameInvocation(relation) {
		return fmt.Errorf("%w: process %q tool call %q changed nested child identity", interaction.ErrSuspensionConflict, processID, relation.ToolCallID)
	}
	for callID, current := range s.pending {
		if callID != relation.ToolCallID && current.ChildID == relation.ChildID {
			return fmt.Errorf("%w: child %q is already owned by tool call %q", interaction.ErrSuspensionConflict, relation.ChildID, callID)
		}
	}
	for callID, current := range s.staged {
		if callID != relation.ToolCallID && current.ChildID == relation.ChildID {
			return fmt.Errorf("%w: child %q is already staged by tool call %q", interaction.ErrSuspensionConflict, relation.ChildID, callID)
		}
	}
	if s.staged == nil {
		s.staged = make(map[string]*nestedChildRelation)
	}
	s.staged[relation.ToolCallID] = relation.clone()
	return nil
}

func (s *nestedChildState) relations() map[string]*nestedChildRelation {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := make(map[string]*nestedChildRelation, len(s.pending)+len(s.staged))
	for callID, relation := range s.pending {
		current[callID] = relation.clone()
	}
	for callID, relation := range s.staged {
		// A child may pause again while resuming the same ToolCall. Its staged
		// relation is the continuation the next checkpoint must persist.
		current[callID] = relation.clone()
	}
	return current
}

func (s *nestedChildState) replacePending(relations []*nestedChildRelation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = make(map[string]*nestedChildRelation, len(relations))
	for _, relation := range relations {
		s.pending[relation.ToolCallID] = relation.clone()
	}
	s.staged = nil
}

func (s *nestedChildState) claim(processID, toolCallID, childID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	relation := s.pending[toolCallID]
	if relation == nil || relation.ChildID != childID {
		return fmt.Errorf("%w: process %q has no pending child %q for tool call %q", interaction.ErrSuspensionStale, processID, childID, toolCallID)
	}
	delete(s.pending, toolCallID)
	delete(s.staged, toolCallID)
	return nil
}

func (s *nestedChildState) unstage(toolCallID, childID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	relation := s.staged[toolCallID]
	if relation == nil || relation.ChildID != childID {
		return false
	}
	delete(s.staged, toolCallID)
	return true
}

func (s *nestedChildState) takeStagedChildIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var childIDs []string
	for _, relation := range s.staged {
		childIDs = append(childIDs, relation.ChildID)
	}
	s.staged = nil
	slices.Sort(childIDs)
	return childIDs
}

func (s *nestedChildState) queueCleanup(childID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.cleanup, childID) {
		s.cleanup = append(s.cleanup, childID)
	}
}

func (s *nestedChildState) takeCleanup() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cleanup := slices.Clone(s.cleanup)
	s.cleanup = nil
	return cleanup
}

func (p *Process) stageNestedChild(relation *nestedChildRelation) error {
	if p == nil {
		return errors.New("runtime: cannot stage nested child on nil parent")
	}
	return p.nested.stage(p.ID(), relation)
}

func (p *Process) nestedChildrenForCheckpoint(
	checkpoint *toolloop.Checkpoint,
) ([]*nestedChildRelation, *nestedChildRelation, error) {
	calls, err := checkpoint.ToolCalls()
	if err != nil {
		return nil, nil, err
	}
	current := p.nested.relations()
	relations := make([]*nestedChildRelation, 0, len(current))
	for _, call := range calls {
		if relation := current[call.ID]; relation != nil {
			relations = append(relations, relation)
			delete(current, call.ID)
		}
	}
	if len(current) != 0 {
		callIDs := slices.Sorted(maps.Keys(current))
		return nil, nil, fmt.Errorf("%w: nested child tool call %q is absent from checkpoint", interaction.ErrSuspensionConflict, callIDs[0])
	}
	active, err := validateCheckpointNestedChildren(checkpoint, relations)
	if err != nil {
		return nil, nil, err
	}
	return relations, active, nil
}

func (p *Process) prepareNestedSuspension(suspension interaction.Suspension) (nestedChildCheckpoint, error) {
	checkpoint, err := nestedChildrenFromSuspension(&suspension)
	if err != nil {
		return nestedChildCheckpoint{}, err
	}
	current := p.nested.relations()
	if len(current) != len(checkpoint.relations) {
		return nestedChildCheckpoint{}, fmt.Errorf(
			"%w: suspension has %d nested children; process has %d",
			interaction.ErrSuspensionConflict,
			len(checkpoint.relations),
			len(current),
		)
	}
	for _, relation := range checkpoint.relations {
		staged := current[relation.ToolCallID]
		if staged == nil || !staged.same(relation) {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: nested child for tool call %q does not match suspension checkpoint", interaction.ErrSuspensionConflict, relation.ToolCallID)
		}
	}
	return checkpoint, nil
}

func (p *Process) commitNestedSuspension(checkpoint nestedChildCheckpoint) {
	if p == nil {
		return
	}
	p.nested.replacePending(checkpoint.relations)
}

func (p *Process) restoreNestedSuspension(suspension *interaction.Suspension) error {
	if p == nil {
		return nil
	}
	checkpoint, err := nestedChildrenFromSuspension(suspension)
	if err != nil {
		return err
	}
	p.nested.replacePending(checkpoint.relations)
	return nil
}

func (p *Process) claimNestedChild(toolCallID, childID string) error {
	if p == nil {
		return errors.New("runtime: cannot claim nested child on nil parent")
	}
	return p.nested.claim(p.ID(), toolCallID, childID)
}

func (p *Process) unstageNestedChild(toolCallID, childID string) bool {
	if p == nil {
		return false
	}
	return p.nested.unstage(toolCallID, childID)
}

func (p *Process) abortStagedNestedChildren(ctx context.Context) int {
	if p == nil {
		return 0
	}
	childIDs := p.nested.takeStagedChildIDs()
	if p.engine == nil {
		return len(childIDs)
	}
	for _, childID := range childIDs {
		_ = p.engine.KillContext(ctx, childID)
		p.engine.discardProcessTree(ctx, childID)
	}
	return len(childIDs)
}

func (p *Process) deferNestedChildCleanup(childID string) {
	if p == nil || childID == "" {
		return
	}
	p.nested.queueCleanup(childID)
}

func (p *Process) takeNestedChildCleanup() []string {
	if p == nil {
		return nil
	}
	return p.nested.takeCleanup()
}
