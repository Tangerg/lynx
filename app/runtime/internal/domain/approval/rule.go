package approval

import (
	"encoding/json"
	"hash/fnv"
	"path"
	"strconv"
	"strings"
)

// Rule matching is a pure function of the candidate rules + the call. Keeping
// it free of I/O lets the store stay a dumb CRUD SPI and lets the precedence
// logic be unit-tested directly.

// subjectOf extracts the per-tool "subject" a rule matches against — the part
// of a call that actually distinguishes one invocation from another. For a
// shell tool that's the command; for a file tool it's the path. Tools with no
// natural sub-subject (most) return "" so a rule for them is whole-tool.
//
// This is the one spot in the approval domain that knows tool argument shapes;
// it mirrors the small per-tool maps elsewhere (e.g. the activity-verb map) and
// stays a closed set — an unknown tool just gets a whole-tool ("") subject.
func subjectOf(tool, argsJSON string) string {
	var field string
	switch tool {
	case "shell", "run_in_background":
		field = "command"
	case "read", "write", "edit":
		field = "file_path"
	default:
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

// hasGlob reports whether a subject pattern carries glob metacharacters.
func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// subjectMatches reports whether a rule's Subject pattern matches a call's
// subject. Empty pattern matches anything (a whole-tool rule); a literal
// pattern matches exactly; a glob pattern matches via path.Match (so `npm run
// *` or `src/*.go` work — note `*` does NOT cross `/`, and `**` is unsupported).
func subjectMatches(pattern, subject string) bool {
	switch {
	case pattern == "":
		return true
	case !hasGlob(pattern):
		return pattern == subject
	default:
		ok, err := path.Match(pattern, subject)
		return err == nil && ok
	}
}

// specificity scores how narrowly a rule targets a call, so the most specific
// matching rule wins. Scope dominates (a session rule beats a project rule
// beats a global one); within a scope an exact subject beats a glob beats a
// whole-tool ("") rule.
func specificity(r Rule) int {
	score := 0
	switch r.Scope {
	case ScopeSession:
		score = 300
	case ScopeProject:
		score = 200
	case ScopeGlobal:
		score = 100
	}
	switch {
	case r.Subject == "":
		score += 0
	case hasGlob(r.Subject):
		score += 1
	default:
		score += 2
	}
	return score
}

// decide picks the verdict for a call from already scope-filtered candidate
// rules (the store returns only rules visible from the call's session/project).
// The most specific matching rule wins; if the top specificity has conflicting
// decisions, Deny wins (a remembered deny must not be overridden by an
// equally-specific allow). ok=false when nothing matches.
func decide(candidates []Rule, q Query) (Decision, bool) {
	subject := subjectOf(q.Tool, q.Arguments)
	best := -1
	var verdict Decision
	conflict := false
	for _, r := range candidates {
		if r.Tool != q.Tool || !subjectMatches(r.Subject, subject) {
			continue
		}
		switch score := specificity(r); {
		case score > best:
			best, verdict, conflict = score, r.Decision, false
		case score == best && r.Decision != verdict:
			conflict = true
		}
	}
	if best < 0 {
		return "", false
	}
	if conflict {
		return Deny, true
	}
	return verdict, true
}

// scopeKey resolves the storage key for a scope; ok=false when the key would be
// empty for a scope that needs one (so the caller drops the rule rather than
// storing an un-keyed session/project rule that would leak across boundaries).
func scopeKey(scope Scope, sessionID, projectDir string) (string, bool) {
	switch scope {
	case ScopeSession:
		return sessionID, sessionID != ""
	case ScopeProject:
		return projectDir, projectDir != ""
	case ScopeGlobal:
		return "", true
	default:
		return "", false
	}
}

// ruleID is a deterministic id over a rule's identity (scope, key, tool,
// subject) so re-remembering the same rule upserts the same row (only the
// decision changes) instead of piling duplicates — and the management UI can
// forget it by a stable id.
func ruleID(scope Scope, scopeKey, tool, subject string) string {
	h := fnv.New64a()
	for _, part := range []string{string(scope), scopeKey, tool, subject} {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "rule_" + strconv.FormatUint(h.Sum64(), 16)
}
