// Package engine integrates lynx-agent + lynx-core into Lyra's
// product-grade runtime. The engine constructs the underlying
// *core.Agent, registers extensions (event listener, tool resolver,
// approval gate, etc.), and exposes a small Go API the service
// implementations consume.
//
// Engine is owned by [New]; everything else is internal plumbing.
// The engine is process-scoped — one *Engine per `lyra serve`.
package engine
