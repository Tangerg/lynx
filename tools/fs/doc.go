// Package fs exposes LLM-callable filesystem tools (read, write, edit,
// glob, grep) on top of a small [Executor] SPI. Local, sandbox, and
// remote backends each implement the SPI; the tools themselves are
// thin adapters that marshal LLM JSON into [Executor] calls and back.
//
// **Text files only.** Executor implementations MUST reject files
// that look binary (NUL byte in the first 8 KiB is a good default
// heuristic) and reject Write content that contains NUL bytes. Use
// the bash tool if you need to manipulate binary data.
//
// **Tools stay thin.** All content processing — line windowing,
// binary detection, exact / fuzzy match, append-vs-overwrite — lives
// in the executor, not the tool. The tool's job is JSON in, JSON out.
//
// Why Glob and Grep live in the SPI (instead of "walk + match" in the
// tool layer): a remote backend cannot afford to ship every file
// across the wire to pattern-match on the agent side. Pushing bulk
// queries into the SPI keeps remote impls one round-trip per call.
package fs
