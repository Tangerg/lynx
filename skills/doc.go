// Package skills is a read-only repository over Agent Skills
// (https://agentskills.io) — directories that each hold a SKILL.md
// (YAML frontmatter + Markdown instructions) plus optional bundled resources
// under references/, assets/, and scripts/.
//
// It exposes [Source] for List/Load and [ResourceSource] for bundled files
// read on demand. [NewFS] wraps any fs.FS; [Dir] is the convenience
// constructor over a real directory.
//
// The package is deliberately minimal: it parses, validates, and serves skill
// content. It does NOT execute scripts — an agent runs those with its own
// shell/file tools — and it does NOT know about chat models or tools. The
// LLM-callable wrapper lives in tools/skills, a thin adapter over
// ResourceSource.
package skills
