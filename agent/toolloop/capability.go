package toolloop

import "github.com/Tangerg/lynx/tools"

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

// Capability finds T on tool, or on the innermost tool that has it, following
// [WrappingTool]. The outermost implementation wins, so a decorator can still
// override what it wraps — that is how [Direct] reports a direct return for a
// tool that does not.
func Capability[T any](tool tools.Tool) (T, bool) {
	for current := tool; !valueIsNil(current); {
		if capability, ok := any(current).(T); ok {
			return capability, true
		}
		wrapper, ok := current.(WrappingTool)
		if !ok {
			break
		}
		current = wrapper.Unwrap()
	}
	var zero T
	return zero, false
}
