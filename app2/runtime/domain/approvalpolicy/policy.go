// Package approvalpolicy owns the Runtime's default effect stance and durable
// remembered decisions. Interrupts remain one-shot interaction facts; this
// package owns only policy that can affect a later tool call.
package approvalpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxIdentityBytes = 512
	maxSubjectBytes  = 16 << 10
)

var ErrInvalid = errors.New("approvalpolicy: invalid policy")

type Mode string

const (
	ModeSafe     Mode = "safe"
	ModeBalanced Mode = "balanced"
	ModeYolo     Mode = "yolo"
)

func (value Mode) Valid() bool {
	return value == ModeSafe || value == ModeBalanced || value == ModeYolo
}

type Effect string

const (
	EffectSafe    Effect = "safe"
	EffectWrite   Effect = "write"
	EffectExec    Effect = "exec"
	EffectNetwork Effect = "network"
)

func (value Mode) RequiresApproval(effect Effect) bool {
	if !effect.Valid() {
		return true
	}
	switch value {
	case ModeSafe:
		return effect != EffectSafe
	case ModeBalanced:
		return effect == EffectExec
	case ModeYolo:
		return false
	default:
		return true
	}
}

func (value Effect) Valid() bool {
	return value == EffectSafe || value == EffectWrite || value == EffectExec || value == EffectNetwork
}

type Scope string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type MatchKind string

const (
	MatchExact MatchKind = "exact"
	MatchGlob  MatchKind = "glob"
)

// RuleState is the storage adapter representation. ScopeKey is deliberately
// private to the wire: Session and project ownership are policy inputs, not a
// second client-controlled scope selector.
type RuleState struct {
	ID        string
	Scope     Scope
	ScopeKey  string
	Tool      string
	Subject   string
	MatchKind MatchKind
	Decision  Decision
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Rule struct{ state RuleState }

type Remember struct {
	Scope      Scope
	SessionID  string
	ProjectDir string
	Tool       string
	Subject    string
	Decision   Decision
	Now        time.Time
}

func NewRemembered(command Remember) (Rule, error) {
	key, err := scopeKey(command.Scope, command.SessionID, command.ProjectDir)
	if err != nil {
		return Rule{}, err
	}
	now := command.Now.UTC()
	state := RuleState{
		Scope: command.Scope, ScopeKey: key, Tool: command.Tool,
		Subject: command.Subject, MatchKind: MatchExact,
		Decision: command.Decision, CreatedAt: now, UpdatedAt: now,
	}
	state.ID = ruleID(state)
	return Rehydrate(state)
}

func Rehydrate(state RuleState) (Rule, error) {
	state.CreatedAt = state.CreatedAt.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	if err := validateState(state); err != nil {
		return Rule{}, err
	}
	return Rule{state: state}, nil
}

func validateState(state RuleState) error {
	switch state.Scope {
	case ScopeSession:
		if !validIdentity(state.ScopeKey) {
			return fmt.Errorf("%w: session scope requires an identity", ErrInvalid)
		}
	case ScopeProject:
		if !filepath.IsAbs(state.ScopeKey) || filepath.Clean(state.ScopeKey) != state.ScopeKey {
			return fmt.Errorf("%w: project scope requires a canonical absolute directory", ErrInvalid)
		}
	case ScopeGlobal:
		if state.ScopeKey != "" {
			return fmt.Errorf("%w: global scope cannot carry an identity", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown scope", ErrInvalid)
	}
	if !validIdentity(state.Tool) {
		return fmt.Errorf("%w: tool identity is unsafe", ErrInvalid)
	}
	if len(state.Subject) > maxSubjectBytes || !utf8.ValidString(state.Subject) || strings.ContainsRune(state.Subject, 0) {
		return fmt.Errorf("%w: subject is unsafe or too large", ErrInvalid)
	}
	if state.Decision != DecisionAllow && state.Decision != DecisionDeny {
		return fmt.Errorf("%w: unknown decision", ErrInvalid)
	}
	if state.MatchKind != MatchExact && state.MatchKind != MatchGlob {
		return fmt.Errorf("%w: unknown match kind", ErrInvalid)
	}
	if state.MatchKind == MatchGlob {
		if _, err := path.Match(state.Subject, ""); err != nil {
			return fmt.Errorf("%w: invalid subject glob", ErrInvalid)
		}
	}
	if state.CreatedAt.IsZero() || state.UpdatedAt.Before(state.CreatedAt) {
		return fmt.Errorf("%w: invalid rule timestamps", ErrInvalid)
	}
	if state.ID == "" || state.ID != ruleID(state) {
		return fmt.Errorf("%w: rule identity does not match its policy key", ErrInvalid)
	}
	return nil
}

type Query struct {
	SessionID  string
	ProjectDir string
	Tool       string
	Subject    string
}

// Decide selects the most specific visible rule. Equal-specificity conflict
// fails closed to deny, independent of storage order.
func Decide(rules []Rule, query Query) (Decision, bool, error) {
	if !validIdentity(query.SessionID) || !filepath.IsAbs(query.ProjectDir) ||
		filepath.Clean(query.ProjectDir) != query.ProjectDir || !validIdentity(query.Tool) ||
		len(query.Subject) > maxSubjectBytes ||
		!utf8.ValidString(query.Subject) || strings.ContainsRune(query.Subject, 0) {
		return "", false, fmt.Errorf("%w: invalid policy query", ErrInvalid)
	}
	best := -1
	var selected Decision
	conflict := false
	for _, rule := range rules {
		if err := validateState(rule.state); err != nil {
			return "", false, err
		}
		if !rule.visibleTo(query) || rule.state.Tool != query.Tool || !rule.matches(query.Subject) {
			continue
		}
		score := rule.specificity()
		switch {
		case score > best:
			best, selected, conflict = score, rule.state.Decision, false
		case score == best && selected != rule.state.Decision:
			conflict = true
		}
	}
	if best < 0 {
		return "", false, nil
	}
	if conflict {
		return DecisionDeny, true, nil
	}
	return selected, true, nil
}

func (value Rule) visibleTo(query Query) bool {
	switch value.state.Scope {
	case ScopeSession:
		return value.state.ScopeKey == query.SessionID
	case ScopeProject:
		return value.state.ScopeKey == query.ProjectDir
	case ScopeGlobal:
		return true
	default:
		return false
	}
}

func (value Rule) matches(subject string) bool {
	if value.state.MatchKind == MatchExact {
		return value.state.Subject == subject
	}
	matched, err := path.Match(value.state.Subject, subject)
	return err == nil && matched
}

func (value Rule) specificity() int {
	score := map[Scope]int{ScopeGlobal: 100, ScopeProject: 200, ScopeSession: 300}[value.state.Scope]
	if value.state.Subject == "" {
		return score
	}
	if value.state.MatchKind == MatchExact {
		return score + 2
	}
	return score + 1
}

func (value Rule) State() RuleState   { return value.state }
func (value Rule) ID() string         { return value.state.ID }
func (value Rule) Scope() Scope       { return value.state.Scope }
func (value Rule) ScopeKey() string   { return value.state.ScopeKey }
func (value Rule) Tool() string       { return value.state.Tool }
func (value Rule) Subject() string    { return value.state.Subject }
func (value Rule) Decision() Decision { return value.state.Decision }

func scopeKey(scope Scope, sessionID, projectDir string) (string, error) {
	switch scope {
	case ScopeSession:
		if !validIdentity(sessionID) {
			return "", fmt.Errorf("%w: session scope has no identity", ErrInvalid)
		}
		return sessionID, nil
	case ScopeProject:
		if !filepath.IsAbs(projectDir) {
			return "", fmt.Errorf("%w: project scope has no absolute directory", ErrInvalid)
		}
		return filepath.Clean(projectDir), nil
	case ScopeGlobal:
		return "", nil
	default:
		return "", fmt.Errorf("%w: unknown scope", ErrInvalid)
	}
}

func ruleID(state RuleState) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		string(state.Scope), state.ScopeKey, state.Tool, state.Subject, string(state.MatchKind),
	}, "\x00")))
	return "rule_" + hex.EncodeToString(digest[:12])
}

func validIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maxIdentityBytes &&
		utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
