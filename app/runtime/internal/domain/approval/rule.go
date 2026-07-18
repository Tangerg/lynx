package approval

import (
	"fmt"
	"hash/fnv"
	"path"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type ruleSet []Rule

// NewRule constructs one durable rule and derives its deterministic identity.
func NewRule(scope Scope, scopeKey, toolName, subject string, decision Decision) (Rule, error) {
	rule := Rule{
		Scope: scope, ScopeKey: scopeKey, Tool: toolName,
		Subject: subject, Decision: decision,
	}
	rule.ID = rule.stableID()
	if err := rule.Validate(); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// Validate protects the durable approval vocabulary and deterministic rule
// identity at every store boundary.
func (r Rule) Validate() error {
	if !r.Scope.Valid() {
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidRule, r.Scope)
	}
	switch r.Scope {
	case ScopeSession, ScopeProject:
		if strings.TrimSpace(r.ScopeKey) == "" {
			return fmt.Errorf("%w: scope %q requires a key", ErrInvalidRule, r.Scope)
		}
	case ScopeGlobal:
		if r.ScopeKey != "" {
			return fmt.Errorf("%w: global scope cannot carry a key", ErrInvalidRule)
		}
	}
	if strings.TrimSpace(r.Tool) == "" || strings.TrimSpace(r.Tool) != r.Tool {
		return fmt.Errorf("%w: tool name is required without surrounding whitespace", ErrInvalidRule)
	}
	if !r.Decision.Valid() {
		return fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, r.Decision)
	}
	if hasGlob(r.Subject) {
		if _, err := path.Match(r.Subject, ""); err != nil {
			return fmt.Errorf("%w: invalid subject glob %q: %w", ErrInvalidRule, r.Subject, err)
		}
	}
	if r.ID == "" || r.ID != r.stableID() {
		return fmt.Errorf("%w: identity %q does not match rule contents", ErrInvalidRule, r.ID)
	}
	return nil
}

// subject extracts the per-tool identity fragment a remembered rule matches:
// shell command, file path, or whole-tool ("") for tools without a finer key.
func (q Query) subject() (string, error) {
	var field string
	switch q.Tool {
	case "shell", "run_in_background":
		field = "command"
	case "read", "write", "edit", "download":
		field = "file_path"
	default:
		return "", nil
	}
	subject, ok := q.Arguments.StringField(field)
	if !ok || strings.TrimSpace(subject) == "" {
		return "", fmt.Errorf("%w: tool %q requires non-empty string argument %q: %w", ErrInvalidQuery, q.Tool, field, tool.ErrInvalidArguments)
	}
	return subject, nil
}

// hasGlob reports whether a subject pattern carries glob metacharacters.
func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
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
func (rs ruleSet) decide(q Query) (Decision, bool, error) {
	if strings.TrimSpace(q.Tool) == "" || strings.TrimSpace(q.Tool) != q.Tool {
		return "", false, fmt.Errorf("%w: tool name is required without surrounding whitespace", ErrInvalidQuery)
	}
	subject, err := q.subject()
	if err != nil {
		return "", false, err
	}
	best := -1
	var verdict Decision
	conflict := false
	for index, r := range rs {
		if err := r.Validate(); err != nil {
			return "", false, fmt.Errorf("approval: candidate rule %d: %w", index, err)
		}
		if r.Tool != q.Tool || !r.matchesSubject(subject) {
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
		return "", false, nil
	}
	if conflict {
		return Deny, true, nil
	}
	return verdict, true, nil
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

func (req RememberRequest) rule() (Rule, error) {
	key, ok := req.Scope.key(req.SessionID, req.ProjectDir)
	if !ok {
		return Rule{}, fmt.Errorf("%w: scope %q has no usable key", ErrInvalidRule, req.Scope)
	}
	if strings.TrimSpace(req.Tool) == "" || strings.TrimSpace(req.Tool) != req.Tool {
		return Rule{}, fmt.Errorf("%w: tool name is required without surrounding whitespace", ErrInvalidRule)
	}
	if !req.Decision.Valid() {
		return Rule{}, fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, req.Decision)
	}
	subject, err := (Query{Tool: req.Tool, Arguments: req.Arguments}).subject()
	if err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}
	return NewRule(req.Scope, key, req.Tool, subject, req.Decision)
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
