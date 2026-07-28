package tools

import (
	"errors"
	"fmt"
)

// ErrInvalidWrappingChain reports a malformed Tool decorator chain.
var ErrInvalidWrappingChain = errors.New("tools: invalid wrapping chain")

// WrappingTool is implemented by a decorator that stands in for another tool.
// Optional capabilities are resolved through this chain, so a decorator states
// once that it wraps a tool instead of re-implementing every optional interface
// the inner tool may acquire.
type WrappingTool interface {
	Unwrap() Tool
}

// maxWrappingDepth bounds malformed chains without requiring Tool values to be
// comparable. A Tool may be a struct containing maps or slices, so generic
// interface equality is not a safe cycle detector.
const maxWrappingDepth = 64

// Capability finds T on tool or through its wrapping chain. The outermost
// implementation wins. Malformed chains and panicking Unwrap implementations
// are returned as errors; capability discovery never uses panic as a hidden
// error channel.
func Capability[T any](tool Tool) (capability T, found bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var zero T
			capability = zero
			found = false
			err = fmt.Errorf("%w: %T Unwrap panicked: %v", ErrInvalidWrappingChain, tool, recovered)
		}
	}()

	current := tool
	for depth := 0; !nilTool(current); depth++ {
		if capability, ok := any(current).(T); ok {
			return capability, true, nil
		}
		wrapper, ok := current.(WrappingTool)
		if !ok {
			break
		}
		if depth == maxWrappingDepth {
			var zero T
			return zero, false, fmt.Errorf(
				"%w: %T exceeds %d wrappers",
				ErrInvalidWrappingChain,
				tool,
				maxWrappingDepth,
			)
		}
		current = wrapper.Unwrap()
	}
	var zero T
	return zero, false, nil
}
