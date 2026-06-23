// Package skills exposes a single LLM-callable tool that surfaces Agent Skills
// to a chat model through progressive disclosure. It is a thin adapter over
// the skills module's [github.com/Tangerg/lynx/skills.Source] capability: the
// base module parses, validates, and serves skill content; this package maps
// that onto the [chat.Tool] contract.
//
// The tool multiplexes three operations on its op argument:
//
//   - list          — every skill's name + description (so the model can pick)
//   - load          — one skill's full instruction body, by name
//   - load_resource — a bundled file under a skill (references/, assets/, scripts/)
//
// Scripts bundled with a skill are NOT executed here — the model runs them
// with its own shell/file tools after reading the instructions.
package skills
