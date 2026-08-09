package interaction

import (
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
)

// ActiveDelegateChild is the immutable Interaction-owned attribution of one
// model ToolCall to its currently active managed child Process. It contains no
// Engine handle, persistence identity, or Host metadata.
type ActiveDelegateChild struct {
	modelCallSequence uint32
	toolCallIndex     uint32
	toolCall          chat.ToolCall
	childKey          agent.ChildKey
	processID         agent.ProcessID
}

// ModelCallSequence returns the one-based model call that requested the child.
func (child ActiveDelegateChild) ModelCallSequence() uint32 {
	return child.modelCallSequence
}

// ToolCallIndex returns the zero-based ToolCall position in the model response.
func (child ActiveDelegateChild) ToolCallIndex() uint32 { return child.toolCallIndex }

// ToolCall returns the exact model ToolCall represented by the child.
func (child ActiveDelegateChild) ToolCall() chat.ToolCall { return child.toolCall }

// ChildKey returns the parent-scoped logical child identity.
func (child ActiveDelegateChild) ChildKey() agent.ChildKey { return child.childKey }

// ProcessID returns the Engine-minted child Process identity.
func (child ActiveDelegateChild) ProcessID() agent.ProcessID { return child.processID }

// Valid reports whether child contains one complete, internally consistent
// active Delegate attribution.
func (child ActiveDelegateChild) Valid() bool {
	if child.modelCallSequence == 0 || child.toolCall.Validate() != nil ||
		!child.childKey.Valid() || !child.processID.Valid() {
		return false
	}
	key, err := DelegateChildKey(child.modelCallSequence, child.toolCall)
	return err == nil && key == child.childKey
}

// ActiveDelegateChildrenFromSnapshot interprets only Interaction-owned state.
// A valid snapshot without an active Interaction Delegate segment returns
// found=false. Returned children preserve model ToolCall order.
func ActiveDelegateChildrenFromSnapshot(
	snapshot agent.Snapshot,
) (children []ActiveDelegateChild, found bool, err error) {
	if !snapshot.Valid() {
		return nil, false, fmt.Errorf("%w: invalid Process snapshot", ErrInvalidExecutionState)
	}
	stateEnvelope := snapshot.CommittedExecutionState()
	if stateEnvelope.Kind() != executionStateKind ||
		stateEnvelope.SchemaVersion() != executionStateSchemaVersion {
		return nil, false, nil
	}
	var state executionState
	if decodeErr := decodeStrict(stateEnvelope.Payload(), &state); decodeErr != nil {
		return nil, false, fmt.Errorf("%w: decode state: %w", ErrInvalidExecutionState, decodeErr)
	}
	if state.DelegateSegment == nil {
		return nil, false, nil
	}
	activeCalls, activeErr := state.activeDelegateCalls()
	if activeErr != nil {
		return nil, false, fmt.Errorf("%w: active Delegate children: %w", ErrInvalidExecutionState, activeErr)
	}
	children = make([]ActiveDelegateChild, 0, len(activeCalls))
	for index, invocation := range state.DelegateSegment.Invocations {
		if invocation.ChildProcessID == nil {
			continue
		}
		if invocation.ChildKey == nil || invocation.ToolResult != nil {
			return nil, false, ErrInvalidExecutionState
		}
		call := activeCalls[index]
		child := ActiveDelegateChild{
			modelCallSequence: state.ModelCallCount,
			toolCallIndex:     state.NextToolCallIndex + uint32(index),
			toolCall:          call,
			childKey:          *invocation.ChildKey,
			processID:         *invocation.ChildProcessID,
		}
		if !child.Valid() {
			return nil, false, fmt.Errorf(
				"%w: Delegate child %d has inconsistent identity", ErrInvalidExecutionState, index,
			)
		}
		children = append(children, child)
	}
	return children, true, nil
}
