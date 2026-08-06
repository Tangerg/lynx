package interaction

// DirectResultTool is an optional Tool capability declaring that a successful
// model-requested batch containing only such tools returns its ordered results
// directly instead of making another model call. The declaration is frozen by
// NewDispatcher; a panic or capability-resolution error rejects construction.
type DirectResultTool interface {
	ReturnsDirectResult() bool
}

// ConcurrentTool is an optional Tool capability declaring which calls are safe
// to overlap within one model-requested batch. Tools without this capability,
// or calls returning concurrent=false, execute alone. A non-empty key names a
// mutually exclusive resource: calls with the same key in that batch never
// overlap. Cross-Process resource coordination remains the Tool owner's job.
//
// Returning concurrent=true also asserts that this invocation will not request
// external input through [RequireToolInput]. A parallel invocation that breaks
// that assertion makes the whole Tool Effect outcome unknown; the Dispatcher
// never re-executes siblings whose side effects may already have happened.
type ConcurrentTool interface {
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}
