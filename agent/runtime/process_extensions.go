package runtime

import "github.com/Tangerg/lynx/agent/core"

// engineExtensions exposes the engine-scoped extension list.
func (p *Process) engineExtensions() []core.Extension {
	if p.engine == nil {
		return nil
	}
	return p.engine.extensions.list
}

// processExtensions exposes the per-process extension list (from
// [core.ProcessOptions.Extensions]).
func (p *Process) processExtensions() []core.Extension {
	if p.options == nil {
		return nil
	}
	return p.options.Extensions
}

// combinedExtensions returns engine extensions followed by process
// extensions — the natural ordering for onion / wrap chains where
// engine sits outermost (registered earliest) and process sits
// innermost (registered last). Goal-approver dispatch reads this list.
func (p *Process) combinedExtensions() []core.Extension {
	return mergeExtensions(p.engineExtensions(), p.processExtensions())
}

// combinedExtensionsResolverFirst returns process extensions BEFORE
// engine extensions — the order used for first-hit resolvers so a
// process-scope override is consulted first.
func (p *Process) combinedExtensionsResolverFirst() []core.Extension {
	return mergeExtensions(p.processExtensions(), p.engineExtensions())
}

// mergeExtensions concatenates first then second, returning the input
// directly (no allocation) when either side is empty.
func mergeExtensions(first, second []core.Extension) []core.Extension {
	if len(second) == 0 {
		return first
	}
	if len(first) == 0 {
		return second
	}
	merged := make([]core.Extension, 0, len(first)+len(second))
	merged = append(merged, first...)
	merged = append(merged, second...)
	return merged
}
