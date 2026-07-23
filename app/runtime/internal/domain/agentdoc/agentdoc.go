// Package agentdoc is the AGENTS.md prompt-source model: the discovered-file
// value type consumed by the workspace and agent-execution adapters. File
// discovery and prompt rendering are adapter responsibilities.
//
// The AGENTS.md convention (https://agents.md) is a cross-tool markdown file
// checked into a repo that briefs any AI coding agent about the project: stack,
// conventions, gotchas, commands. Nested notes take precedence over notes at the
// repo root.
//
// LYRA.md is intentionally NOT in scope here — that's managed by
// internal/domain/knowledge (writable via the runtime protocol). AGENTS.md is
// read-only at runtime; the engine never writes to it.
package agentdoc

// File is one discovered AGENTS.md (or agents.md) with its absolute path.
type File struct {
	Path    string
	Content string
}
