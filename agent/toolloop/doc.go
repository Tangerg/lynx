// Package toolloop drives model/tool control flow without extending the chat
// protocol with runtime state.
//
// A Runner consumes a core/chat Model and an Invocation, then emits a lazy
// sequence of Events. Model requests and responses remain protocol values;
// executable tools stay in a runtime-only ToolResolver. Tool calls run
// serially by default, ordinary tool errors become error ToolResults, and the
// runner never retries a model or tool call automatically.
//
// Tools request resumable control flow with PauseError. The resulting Pause
// event carries a serializable Checkpoint; Resume continues at the pending
// call without re-invoking the model or completed tools. AbortError and context
// cancellation propagate as errors. Direct marks tools whose all-direct round
// ends on its final ToolResult.
//
// The older NewMiddleware path remains frozen only while existing agent
// consumers still use the legacy chat protocol. It is a separate
// implementation, not an adapter for Runner, and is removed with its final
// consumer during the workspace cutover.
package toolloop
