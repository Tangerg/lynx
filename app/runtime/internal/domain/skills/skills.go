// Package skills is the skill-discovery model: the client-facing Info projection
// and the product vocabulary for an agent-authored skill. Filesystem layout,
// discovery, and SKILL.md encoding belong to their adapters.
package skills

// Info is one discovered skill, tagged with the scope it came from — the
// client-facing projection (skills.discovered.list) of what the merged source
// exposes to the model (same sources, same precedence).
type Info struct {
	Name        string
	Description string
	Scope       string // "project" (<workdir>/.lyra/skills) | "global"
}
