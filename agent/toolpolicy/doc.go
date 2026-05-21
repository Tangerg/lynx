// Package toolpolicy ships [chat.Tool] decorators that enforce
// LLM-tool-loop policies — the runtime equivalent of embabel's
// `agentic/` family (OneShotPerLoopTool / PlaybookTool with
// UnlockCondition / …).
//
// Each helper wraps an existing tool and returns a new
// [chat.Tool]. Compose freely:
//
//	tool := toolpolicy.Unlocked(
//	    toolpolicy.OnceOnly(rawSearch),
//	    func(ctx context.Context) bool {
//	        p := core.ProcessFrom(ctx); return p != nil && approved(p)
//	    },
//	)
//
// The decorators rely only on [context.Context] for state; they do not
// require a parent process to function (though
// [Unlocked]'s condition typically inspects [core.ProcessFrom] for
// blackboard access). Both are goroutine-safe.
package toolpolicy
