// Package knowledge defines Lyra's long-term knowledge values. Human-authored
// knowledge remains the editable LYRA.md cascade; agent-extracted facts live in
// an append-only ledger and are periodically folded into project-scoped curated
// memory. Prompt composition remains in the agent-execution adapter.
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

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

// Store is the long-term knowledge persistence contract. The implementation is
// file-backed (internal/infra/storage) — LYRA.md stays a user-editable
// file by design, the one deliberate exception to the SQLite backend.
// dir on each method is the project directory [ScopeProject] reads
// from / writes to — a session's working directory, so one store
// serves every project. [ScopeUser] ignores it. Empty dir falls back
// to the implementation's default directory (the process cwd for the
// file-backed store), preserving single-project behavior for
// callers with no session in hand (CLI, wire requests without cwd).
type Store interface {
	// Get returns the full LYRA.md content for the given scope.
	// Empty result is valid (file may not exist yet).
	Get(ctx context.Context, scope Scope, dir string) (string, error)

	// Update overwrites the LYRA.md file for the given scope with
	// the supplied markdown. Concurrent writers race; last-write-wins.
	Update(ctx context.Context, scope Scope, dir string, content string) error

	// List enumerates every memory entry across scopes. Used by UIs
	// that want to render a list rather than a flat markdown blob.
	List(ctx context.Context, dir string) ([]Entry, error)
}

// FactBatch is one extraction boundary's project-scoped ledger append.
type FactBatch struct {
	Project    string
	SessionID  string
	Day        string
	Facts      []string
	CapturedAt time.Time
}

// Normalize validates the batch identity and canonicalizes its facts into
// unique markdown bullets while preserving first-seen order.
func (b FactBatch) Normalize() (FactBatch, error) {
	b.Project = strings.TrimSpace(b.Project)
	b.SessionID = strings.TrimSpace(b.SessionID)
	if b.Project == "" {
		return FactBatch{}, errors.New("knowledge: fact batch project is required")
	}
	if b.SessionID == "" {
		return FactBatch{}, errors.New("knowledge: fact batch session is required")
	}
	day, err := time.Parse(time.DateOnly, b.Day)
	if err != nil || day.Format(time.DateOnly) != b.Day {
		return FactBatch{}, fmt.Errorf("knowledge: invalid ledger day %q", b.Day)
	}
	if b.CapturedAt.IsZero() {
		return FactBatch{}, errors.New("knowledge: fact batch capture time is required")
	}
	b.Facts = NormalizeFacts(strings.Join(b.Facts, "\n"))
	return b, nil
}

// LedgerFact is one immutable fact in a project's daily ledger. Sequence is
// the durable ordering key and curation watermark.
type LedgerFact struct {
	Sequence   int64
	Day        string
	Content    string
	CapturedAt time.Time
}

// Curated is the complete agent-maintained project memory visible to prompts.
// Watermark is the highest ledger sequence incorporated into Content.
type Curated struct {
	Content   string
	Watermark int64
	UpdatedAt time.Time
}

// NormalizeFacts converts an extraction response into stable markdown bullets.
// Blank lines, fences, and the NO_FACTS sentinel are discarded; duplicate facts
// within one response collapse without reordering the survivors.
func NormalizeFacts(markdown string) []string {
	var facts []string
	seen := make(map[string]struct{})
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "```" || strings.EqualFold(line, "NO_FACTS") {
			continue
		}
		line = trimBullet(line)
		if line == "" {
			continue
		}
		fact := "- " + line
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}
	return slices.Clip(facts)
}

func trimBullet(line string) string {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && line[1] == ' ' {
		return strings.TrimSpace(line[2:])
	}
	if index := strings.IndexByte(line, '.'); index > 0 && index+1 < len(line) && line[index+1] == ' ' {
		for _, digit := range line[:index] {
			if digit < '0' || digit > '9' {
				return line
			}
		}
		return strings.TrimSpace(line[index+2:])
	}
	return line
}
