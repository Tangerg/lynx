// Package skills is a read-only repository over Agent Skills
// (https://agentskills.io) — directories that each hold a SKILL.md
// (YAML frontmatter + Markdown instructions) plus optional bundled resources
// under references/, assets/, and scripts/.
//
// It exposes one capability, [Source], with three operations mirroring the
// spec's progressive-disclosure levels: List (name + description for every
// skill), Load (one skill's full instructions), and LoadResource (a bundled
// file, read on demand). [FS] implements Source over any fs.FS; [Dir] is the
// convenience constructor over a real directory.
//
// The package is deliberately minimal: it parses, validates, and serves skill
// content. It does NOT execute scripts — an agent runs those with its own
// shell/file tools — and it does NOT know about chat models or tools. The
// LLM-callable wrapper lives in tools/skills, a thin adapter over this Source.
package skills
