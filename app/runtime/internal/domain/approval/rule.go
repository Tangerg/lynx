package approval

import (
	"encoding/json"
	"hash/fnv"
	"path"
	"strconv"
	"strings"
)

type ruleSet []Rule

// subject extracts the per-tool identity fragment a remembered rule matches:
// shell command, file path, or whole-tool ("") for tools without a finer key.
func (q Query) subject() string {
	var field string
	switch q.Tool {
	case "shell", "run_in_background":
		field = "command"
	case "read", "write", "edit", "download":
		field = "file_path"
	default:
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(q.Arguments), &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

// hasGlob reports whether a subject pattern carries glob metacharacters.
func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func (r Rule) matches(q Query) bool {
	return r.Tool == q.Tool && r.matchesSubject(q.subject())
}

// matchesSubject uses path.Match for glob subjects; "*" intentionally does not
// cross "/" and "**" is not special.
func (r Rule) matchesSubject(subject string) bool {
	switch {
	case r.Subject == "":
		return true
	case !hasGlob(r.Subject):
		return r.Subject == subject
	default:
		ok, err := path.Match(r.Subject, subject)
		return err == nil && ok
	}
}

// specificity encodes the conflict policy: narrower scope beats wider scope,
// then exact subject beats glob, and glob beats whole-tool.
func (r Rule) specificity() int {
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

// decide picks the strongest visible rule; equally specific disagreements
// resolve to Deny so a remembered deny cannot be canceled by a peer allow.
func (rs ruleSet) decide(q Query) (Decision, bool) {
	best := -1
	var verdict Decision
	conflict := false
	for _, r := range rs {
		if !r.matches(q) {
			continue
		}
		switch score := r.specificity(); {
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

// key refuses empty session/project keys so rules cannot leak across scopes.
func (s Scope) key(sessionID, projectDir string) (string, bool) {
	switch s {
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

func (req RememberRequest) rule() (Rule, bool) {
	key, ok := req.Scope.key(req.SessionID, req.ProjectDir)
	if !ok {
		return Rule{}, false
	}
	rule := Rule{
		Scope:    req.Scope,
		ScopeKey: key,
		Tool:     req.Tool,
		Subject:  Query{Tool: req.Tool, Arguments: req.Arguments}.subject(),
		Decision: req.Decision,
	}
	rule.ID = rule.stableID()
	return rule, true
}

// stableID makes re-remembering the same rule an upsert and gives the UI a
// durable forget handle.
func (r Rule) stableID() string {
	h := fnv.New64a()
	for _, part := range []string{string(r.Scope), r.ScopeKey, r.Tool, r.Subject} {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "rule_" + strconv.FormatUint(h.Sum64(), 16)
}
