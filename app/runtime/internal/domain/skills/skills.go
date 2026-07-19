// Package skills is the skill-discovery model: the client-facing Info projection
// and the project-over-global precedence rule (the .lyra/skills convention). The
// filesystem discovery — building the merged skill source and listing SKILL.md
// entries — is the promptsource adapter's job (internal/adapter/promptsource),
// not this package's.
package skills

import "path/filepath"

// ProjectSubdir is where per-project skills live, relative to the session's
// working directory — the .lyra/ convention shared with .lyra/AGENTS.md.
// Project skills take precedence over the global set.
const ProjectSubdir = ".lyra/skills"

// Info is one discovered skill, tagged with the scope it came from — the
// client-facing projection (skills.discovered.list) of what the merged source
// exposes to the model (same sources, same precedence).
type Info struct {
	Name        string
	Description string
	Scope       string // "project" (<workdir>/.lyra/skills) | "global"
}

// ProjectDir resolves the project skills directory for a session working
// directory. Empty workdir → empty (no project skills).
func ProjectDir(workdir string) string {
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, ProjectSubdir)
}
