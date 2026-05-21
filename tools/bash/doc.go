// Package bash exposes a single LLM-callable shell tool plus a small
// [Executor] SPI. The package itself does not pick where commands run
// — local, sandboxed, or remote backends each implement [Executor] and
// plug in via [NewTool].
//
// The local executor ([NewLocalExecutor]) is the reference impl and
// covers the common case (run on the same host as the agent).
package bash
