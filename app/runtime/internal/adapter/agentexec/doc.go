// Package kernel integrates lynx-agent + lynx-core into Lyra's
// product-grade runtime. The kernel constructs the underlying
// *core.Agent, registers extensions (event listener, tool resolver,
// the runtime approval-stance tool check, etc.), and exposes a small
// Go API the service implementations consume.
//
// Engine is owned by [New]; everything else is internal plumbing.
// The engine is process-scoped — one *Engine per runtime server process.
package agentexec
