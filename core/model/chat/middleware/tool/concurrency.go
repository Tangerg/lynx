package tool

import "github.com/Tangerg/lynx/core/model/chat"

// ConcurrentTool is the optional capability a tool carries to opt into
// CONCURRENT execution within one tool-calling round. It is a contract between
// a tool and THIS loop driver — deliberately NOT in the core chat package: a
// tool's concurrency-safety is advice a driver MAY honor (run non-conflicting
// calls in parallel) or ignore (run every call serially — always correct), the
// same way [ParkStore] is a capability this driver consumes rather than a core
// protocol type.
//
// A tool opts in by implementing the method on its own type — structurally, so
// it needs NOT import this package (a tool must not depend on a specific loop
// driver) — or, for a tool built via [chat.NewTool] that can't add a method, by
// wrapping with [AsParallelTool]. A tool that doesn't implement it runs
// exclusively (alone).
type ConcurrentTool interface {
	chat.Tool

	// ConcurrencyKey reports, for the call described by arguments, whether the
	// call may overlap other calls in the round and the resource it conflicts
	// on:
	//   - concurrent == false           → run the call ALONE (also the behavior
	//     of a tool that doesn't implement ConcurrentTool) — the safe default
	//     for opaque side effects, shared state, and HITL tools.
	//   - concurrent == true, key == "" → run in parallel; no resource conflict
	//     (a pure read, a network fetch, an isolated sub-agent).
	//   - concurrent == true, key != "" → run in parallel EXCEPT against another
	//     call reporting the same key, which serializes (two edits to the same
	//     file path conflict; edits to distinct files don't).
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}

// AsParallelTool wraps t so it reports itself (via [ConcurrentTool]) as
// concurrency-safe with no resource conflict — the opt-in for a read-only /
// side-effect-free tool built with [chat.NewTool], which can't add the method
// to its own type. A tool that owns its struct should implement
// [ConcurrentTool] directly (e.g. a file-mutating tool returning a per-call
// key) instead.
func AsParallelTool(t chat.Tool) chat.Tool {
	return parallelTool{Tool: t}
}

type parallelTool struct{ chat.Tool }

func (parallelTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }
