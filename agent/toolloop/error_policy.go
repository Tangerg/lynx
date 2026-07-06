package toolloop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// unknownToolResult is the synthetic tool result the invoker feeds back to the
// model when it calls a tool that isn't registered. It names the missing tool
// and lists the invoker's registered tools so the model can recover.
func (i *invoker) unknownToolResult(name string) string {
	available := i.registry.names()
	slices.Sort(available)
	if len(available) == 0 {
		return fmt.Sprintf("error: tool %q is not available, and no tools are registered", name)
	}
	return fmt.Sprintf("error: tool %q is not available. Available tools: %s", name, strings.Join(available, ", "))
}

// toolErrorResult is the synthetic tool result the invoker feeds back to the
// model when a tool execution fails recoverably, so the model sees the failure
// and can adjust instead of the whole request aborting. The error string is
// the tool's own (already wrapped by the tool); the invoker does not add its
// internal call path.
func (i *invoker) toolErrorResult(name string, err error) string {
	return fmt.Sprintf("error: tool %q failed: %s", name, err.Error())
}

// abortsToolLoop reports whether a tool error must PROPAGATE (abort the loop)
// instead of being fed back to the model as a recoverable result. Two cases:
// context cancellation / deadline (the run is being torn down), and a
// [Halt] whose Abort() is true — a fatal failure the model can't fix.
// (A Halt whose Abort() is false is a HITL interrupt — see
// [invoker.interruptsToolLoop] — which also propagates but parks rather
// than fails.) Together with interruptsToolLoop it is the invoker's tool-error
// classification policy; stateless but owned by the invoker that applies it.
func (i *invoker) abortsToolLoop(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	h, ok := errors.AsType[Halt](err)
	return ok && h.Abort()
}

// interruptsToolLoop reports whether a tool error is a human-in-the-loop
// INTERRUPT — a [Halt] whose Abort() is false. The loop stops and propagates it
// unchanged (no feedback to the model) so an outer layer can park the run and
// gather input; on resume the parked tail is fed back and the loop continues AT
// the still-pending call (the model is not re-invoked for that round).
// agent/hitl.InterruptError is the reference implementation; the contract is
// duck-typed so this package never imports agent.
func (i *invoker) interruptsToolLoop(err error) bool {
	h, ok := errors.AsType[Halt](err)
	return ok && !h.Abort()
}
