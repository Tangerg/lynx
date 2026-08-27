package tool

import (
	"errors"
	"fmt"

	"github.com/samber/lo"
)

var ErrInvalidWrappingChain = errors.New("tool: invalid wrapping chain")

// WrappingTool is implemented by a decorator that stands in for another tool.
// Optional capabilities are resolved through this chain, so a decorator states
// once that it wraps a tool instead of re-implementing every optional interface
// the inner tool may acquire.
type WrappingTool interface {
	// Unwrap returns the next inner tool in a finite decorator chain. It must
	// return the same binding for the wrapper's lifetime and must not perform I/O;
	// nil, cycles, excessive depth, and panics make the chain invalid.
	Unwrap() Tool
}

const maxWrappingDepth = 64

// Capability finds T on value or through its wrapping chain. The outermost
// implementation wins. Malformed chains and panicking Unwrap implementations
// are returned as errors.
func Capability[T any](value Tool) (capability T, found bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var zero T
			capability = zero
			found = false
			err = fmt.Errorf("%w: %T Unwrap panicked: %v", ErrInvalidWrappingChain, value, recovered)
		}
	}()

	current := value
	for depth := 0; !lo.IsNil(current); depth++ {
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
				value,
				maxWrappingDepth,
			)
		}
		current = wrapper.Unwrap()
	}
	var zero T
	return zero, false, nil
}
