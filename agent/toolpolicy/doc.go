// Package toolpolicy ships `tools.Tool` toolMiddleware that enforce
// LLM-tool-loop policies — OneShotPerLoop, Playbook with
// UnlockCondition, and similar patterns.
//
// Each helper wraps an existing tool and returns a new `tools.Tool`
// (plus an error for nil inputs). Compose freely:
//
//	once, err := toolpolicy.OnceOnly(rawSearch)
//	if err != nil { /* handle */ }
//	tool, err := toolpolicy.Unlocked(once,
//	    func(ctx context.Context, arguments string) (bool, string) {
//	        if p := core.ProcessViewFrom(ctx); p != nil && approved(p) {
//	            return true, ""
//	        }
//	        return false, "awaiting approval"
//	    },
//	)
//
// The toolMiddleware rely only on [context.Context] for state; they do not
// require a parent process to function (though
// [Unlocked]'s condition typically inspects [core.ProcessViewFrom] for
// blackboard access). Both are goroutine-safe.
package toolpolicy
