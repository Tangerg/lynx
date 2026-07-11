// Package agentdoc is the AGENTS.md prompt-source model: the discovered-file
// value type and the budgeted render rule the engine injects into the system
// prompt. File discovery (walking the project tree + user-level dirs) is the
// filesystem adapter's job (internal/adapter/promptsource), not this package's.
//
// The AGENTS.md convention (https://agents.md) is a cross-tool markdown file
// checked into a repo that briefs any AI coding agent about the project: stack,
// conventions, gotchas, commands. Nested notes take precedence over notes at the
// repo root; [Render] keeps the deepest (most specific) files when the budget is
// tight.
//
// LYRA.md is intentionally NOT in scope here — that's managed by
// internal/domain/knowledge (writable via the runtime protocol). AGENTS.md is
// read-only at runtime; the engine never writes to it.
package agentdoc

import "strings"

// DefaultMaxBytes caps the rendered blob — 32 KiB, generous enough for several
// layers of nested AGENTS.md, tight enough to not blow the system prompt budget.
const DefaultMaxBytes = 32 * 1024

// File is one discovered AGENTS.md (or agents.md) with its absolute path. Path
// is the source-of-truth used in render annotations so the model can attribute
// claims back to the right file.
type File struct {
	Path    string
	Content string
}

// Render concatenates files into a single blob with `<!-- From: /path -->`
// provenance headers. When the byte budget is exceeded, files at the front of
// the list (root-most) are dropped first — they're the least specific so most
// expendable. Returns "" when no files fit or the input is empty.
func Render(files []File, maxBytes int) string {
	if len(files) == 0 || maxBytes <= 0 {
		return ""
	}

	blocks := make([]string, len(files))
	sizes := make([]int, len(files))
	total := 0
	for i, f := range files {
		blocks[i] = annotation(f.Path) + f.Content + "\n"
		sizes[i] = len(blocks[i])
		total += sizes[i]
	}
	// Inter-block separator (one blank line between blocks).
	if len(files) > 1 {
		total += len(files) - 1
	}

	start := 0
	for start < len(files) && total > maxBytes {
		total -= sizes[start]
		if start > 0 {
			total-- // remove the separator that was between start-1 and start
		}
		start++
	}
	if start >= len(files) {
		return ""
	}

	var b strings.Builder
	b.Grow(total)
	for i := start; i < len(files); i++ {
		if i > start {
			b.WriteString("\n")
		}
		b.WriteString(blocks[i])
	}
	return b.String()
}

// annotation is the per-file header. Keep it on one line + trailing LF so the
// model reads the path inline without extra blank.
func annotation(path string) string {
	return "<!-- From: " + path + " -->\n"
}
