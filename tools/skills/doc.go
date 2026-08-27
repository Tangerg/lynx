// Package skills exposes three LLM-callable tools that surface Agent Skills to
// a chat model through progressive disclosure. It is a thin adapter over
// the skills module's [github.com/Tangerg/scope/skills.Source] capability: the
// base module parses, validates, and serves skill content; this package maps
// that onto the shared `tool.Tool` contract.
//
// Each disclosure step has one tool and one input shape:
//
//   - list_skills — every skill's name + description
//   - load_skill — one skill's full instruction body, by name
//   - read_skill_resource — one bundled file under a skill
//
// Scripts bundled with a skill are NOT executed here — the model runs them
// with its own shell/file tools after reading the instructions.
package skills
