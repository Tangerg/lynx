// Package agentexec is the agent-SDK anti-corruption adapter: it integrates
// lynx-agent + lynx-core into Lyra's runtime. It constructs the underlying
// *core.Agent, registers extensions (event listener, tool resolver, the
// runtime approval-stance tool check, etc.), and exposes a small Go API the
// application layer drives to run one segment (a chat turn + its tool loop).
//
// Engine is owned by [New]; everything else is internal plumbing. The engine
// is process-scoped — one *Engine per runtime server process.
package agentexec
