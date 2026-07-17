// Package toolpolicy provides concurrency-safe decorators for tool call
// policies.
//
// [Once] limits a tool to one call per [WithScope] context. [Gate] evaluates a
// [Condition] before every call. The decorators compose without hiding a
// tool's return-direct or file-mutation declarations. They intentionally keep
// calls exclusive because their policy state is part of the ordering contract:
//
//	once, err := toolpolicy.Once(rawSearch)
//	if err != nil { /* handle */ }
//	tool, err := toolpolicy.Gate(once,
//	    func(ctx context.Context, arguments string) (bool, string) {
//	        if approved(ctx) {
//	            return true, ""
//	        }
//	        return false, "awaiting approval"
//	    },
//	)
//
// The package depends only on [context.Context] for call scope and does not
// require an agent process. Both decorators are safe for concurrent use.
package toolpolicy
