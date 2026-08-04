// Package knowledge defines Lyra's human-authored long-term knowledge: the
// user-editable LYRA.md cascade. Agent-maintained memory (the mined fact ledger
// and its curated items) is a separate bounded context — see package
// agentmemory. Prompt composition remains in the agent-execution adapter.
package knowledge

import (
	"fmt"
	"time"
)

// Scope selects which LYRA.md the operation targets. The prompt
// composes both per turn — user (global) first, then project, so
// project knowledge extends and overrides the global preferences.
type Scope string

const (
	// ScopeProject — `<dir>/LYRA.md`. Project-specific knowledge:
	// conventions, key files, gotchas. Addressed by the project
	// directory passed per call (a session's cwd), so one store
	// serves every project.
	ScopeProject Scope = "project"
	// ScopeUser — `~/.lyra/LYRA.md`. Cross-project preferences:
	// coding style, tools, vocabulary. The global scope; per-call
	// dir is ignored.
	ScopeUser Scope = "user"
)

// Valid reports whether s names one of the two human-authored knowledge
// partitions.
func (s Scope) Valid() bool {
	return s == ScopeProject || s == ScopeUser
}

// Validate rejects a value that cannot identify a LYRA.md partition.
func (s Scope) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("knowledge: invalid scope %q", s)
	}
	return nil
}

func (s Scope) String() string { return string(s) }

// Entry is one piece of stored memory. Content is the verbatim markdown
// shown to the model; CapturedAt records when it landed in LYRA.md.
type Entry struct {
	Scope      Scope
	Content    string
	CapturedAt time.Time // when this entry last landed in LYRA.md
}
