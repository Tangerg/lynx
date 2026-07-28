package toolloop

import (
	"fmt"

	"github.com/Tangerg/lynx/tools"
)

// WrappingTool is implemented by a decorator that stands in for another tool.
//
// Optional tool capabilities — [ConcurrentTool], [DeferredTool], the
// direct-return marker, or a host's own — are looked up through this chain, so a
// decorator states once that it wraps something instead of re-implementing every
// capability its inner tool might have. That list only grows, and a decorator
// that misses an entry loses the inner tool's advice silently: exclusive
// execution where concurrency was declared, a full manifest where tools were
// withheld.
//
// Unwrap returns the wrapped tool. A chain that leads back to itself is a
// programming error, exactly as it is for [errors.Unwrap].
type WrappingTool interface {
	Unwrap() tools.Tool
}

// maxWrappingDepth bounds how far a lookup follows [WrappingTool]. Real
// decorator stacks are a handful deep, so a chain this long is one that leads
// back into itself.
//
// The bound is what makes a malformed chain terminate. Comparing tools for
// identity instead would be the direct test, but a tool need not be comparable —
// a struct value holding a map satisfies tools.Tool and panics on ==, turning a
// diagnostic into a different crash.
const maxWrappingDepth = 64

// Capability finds T on tool, or on the innermost tool that has it, following
// [WrappingTool]. The outermost implementation wins, so a decorator can still
// override what it wraps — that is how [Direct] reports a direct return for a
// tool that does not.
//
// A chain that never ends is a decorator bug, and this reports it rather than
// spinning: it panics, which every boundary that invokes a tool converts into an
// error attributed to that tool. Returning "no capability" instead would leave
// the tool silently exclusive, or silently reporting no file mutations, which is
// the kind of quiet wrong answer the chain exists to prevent.
func Capability[T any](tool tools.Tool) (T, bool) {
	current := tool
	for depth := 0; !valueIsNil(current); depth++ {
		if capability, ok := any(current).(T); ok {
			return capability, true
		}
		wrapper, ok := current.(WrappingTool)
		if !ok {
			break
		}
		if depth == maxWrappingDepth {
			panic(fmt.Sprintf(
				"toolloop: %T is %d wrappers deep; its Unwrap chain does not end",
				current,
				maxWrappingDepth,
			))
		}
		current = wrapper.Unwrap()
	}
	var zero T
	return zero, false
}
