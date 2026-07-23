// Package knowledge defines Lyra's human-authored long-term knowledge: the
// user-editable LYRA.md cascade. Agent-maintained memory (the mined fact ledger
// and its curated items) is a separate bounded context — see package
// agentmemory. Prompt composition remains in the agent-execution adapter.
package knowledge

import "time"

// Scope selects which LYRA.md the operation targets. The prompt
// composes both per turn — user (global) first, then project, so
// project knowledge extends and overrides the global preferences.
type Scope int

const (
	// ScopeProject — `<dir>/LYRA.md`. Project-specific knowledge:
	// conventions, key files, gotchas. Addressed by the project
	// directory passed per call (a session's cwd), so one store
	// serves every project.
	ScopeProject Scope = iota
	// ScopeUser — `~/.lyra/LYRA.md`. Cross-project preferences:
	// coding style, tools, vocabulary. The global scope; per-call
	// dir is ignored.
	ScopeUser
)

// Entry is one piece of stored memory. Content is the verbatim markdown
// shown to the model; CapturedAt records when it landed in LYRA.md.
type Entry struct {
	Scope      Scope
	Content    string
	CapturedAt time.Time // when this entry last landed in LYRA.md
}
