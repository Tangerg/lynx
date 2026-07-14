// Package toolpolicy ships `tools.Tool` decorators that enforce
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
//	        if p := core.ProcessFrom(ctx); p != nil && approved(p) {
//	            return true, ""
//	        }
//	        return false, "awaiting approval"
//	    },
//	)
//
// The decorators rely only on [context.Context] for state; they do not
// require a parent process to function (though
// [Unlocked]'s condition typically inspects [core.ProcessFrom] for
// blackboard access). Both are goroutine-safe.
package toolpolicy
