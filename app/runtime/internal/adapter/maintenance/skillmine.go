package maintenance

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const (
	// defaultMinerComplexityThreshold is the minimum completed tool-call count
	// for a turn to count as "complex" enough to consider distilling. Below it a
	// turn is routine and never triggers a mining attempt.
	defaultMinerComplexityThreshold = 8
	// defaultMinerCadence mines at most once per this many complex turns per
	// session, bounding the extra LLM call and avoiding draft spam.
	defaultMinerCadence = 3
	// minerMinMessages skips mining a conversation too short to hold a reusable
	// procedure.
	minerMinMessages = 4
)

// MinerConfig tunes when the [SkillMiner] attempts a distillation. Zero values
// select package defaults.
type MinerConfig struct {
	// ComplexityThreshold is the minimum completed tool-call count for a turn to
	// count as complex. Only complex turns advance the cadence counter.
	ComplexityThreshold int
	// Cadence bounds mining to at most once per this many complex turns, per
	// session.
	Cadence int
}

func (c MinerConfig) normalized() MinerConfig {
	if c.ComplexityThreshold <= 0 {
		c.ComplexityThreshold = defaultMinerComplexityThreshold
	}
	if c.Cadence <= 0 {
		c.Cadence = defaultMinerCadence
	}
	return c
}

// draftStore is the miner's narrow view of the skill-authoring store: it stages
// a proposed draft under the governed _drafts/ area, where it stays invisible to
// the model until a human promotes it. The miner never publishes a skill.
type draftStore interface {
	Enabled() bool
	SaveDraft(ctx context.Context, draft skills.Draft) (skills.DraftHandle, error)
}

// SkillMiner distills a complex turn's trajectory into a proposed skill draft.
// It runs at the turn boundary — after a clean finish, before compaction — and
// takes the Hermes learning-loop's "mine automatically" idea but grounds it in
// the governed B4 write path: every proposal lands in _drafts/ behind the
// mandatory human promotion gate, stamped with agent provenance. The agent
// never publishes a skill on its own. Mining is a direct, middleware-free LLM
// call (like [Extractor]), never a forked agent.
type SkillMiner struct {
	history messageReader
	store   draftStore
	client  ClientFunc
	config  MinerConfig
	minMsgs int

	// mu guards complexTurns, the per-session count of complex turns since the
	// last mining attempt. In-memory and reset on restart: it bounds cost, not a
	// correctness invariant.
	mu           sync.Mutex
	complexTurns map[string]int
}

// NewSkillMiner builds the turn-boundary skill miner over the conversation
// history reader, the authoring store, and the utility-model client resolver.
func NewSkillMiner(history messageReader, store draftStore, client ClientFunc, config MinerConfig) *SkillMiner {
	return &SkillMiner{
		history:      history,
		store:        store,
		client:       client,
		config:       config.normalized(),
		minMsgs:      minerMinMessages,
		complexTurns: map[string]int{},
	}
}

// MaybeMine distills the session's recent trajectory into a proposed skill
// draft when the just-finished turn was complex enough and the per-session
// cadence is due. A distillation that yields no reusable skill, an unparseable
// or invalid document, or an obviously-dangerous one is dropped silently
// (return nil) — only a real read/save/LLM failure surfaces as an error.
func (m *SkillMiner) MaybeMine(ctx context.Context, sessionID, cwd string, toolCalls int) error {
	if m == nil || m.store == nil || !m.store.Enabled() || sessionID == "" || cwd == "" {
		return nil
	}
	if toolCalls < m.config.ComplexityThreshold {
		return nil
	}
	if !m.due(sessionID) {
		return nil
	}
	messages, err := m.history.Read(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("skill mining: read session %q: %w", sessionID, err)
	}
	if len(messages) < m.minMsgs {
		return nil
	}
	document, err := m.askForSkill(ctx, messages)
	if err != nil {
		return fmt.Errorf("skill mining: distill skill: %w", err)
	}
	if document == "" {
		return nil
	}
	front, body, err := skillspec.Parse([]byte(document))
	if err != nil {
		return nil
	}
	draft := skills.Draft{
		Name:          front.Name,
		Description:   front.Description,
		Body:          body,
		CreatedBy:     skills.CreatedByAgent,
		SourceSession: sessionID,
	}
	if err := draft.Validate(); err != nil {
		return nil
	}
	if _, dangerous := draft.Scan(); dangerous {
		return nil
	}
	if _, err := m.store.SaveDraft(ctx, draft); err != nil {
		return fmt.Errorf("skill mining: save draft %q: %w", draft.Name, err)
	}
	return nil
}

// due advances the session's complex-turn counter and reports whether a mining
// attempt is now due, resetting the counter when it fires. Resetting on the
// attempt (not on a successful save) is deliberate: the cadence bounds LLM
// calls, and every due attempt makes one whether or not it yields a draft.
func (m *SkillMiner) due(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.complexTurns[sessionID]++
	if m.complexTurns[sessionID] >= m.config.Cadence {
		m.complexTurns[sessionID] = 0
		return true
	}
	return false
}

// skillMinerPrompt distils the Hermes prompt wisdom (H-Skill-4): prefer a
// reusable, class-level procedure; refuse the well-known anti-patterns. The
// model returns a complete SKILL.md or the exact sentinel NO_SKILL.
const skillMinerPrompt = `You are mining a coding-agent conversation for a REUSABLE skill worth saving for future sessions.

A skill is a class-level, reusable procedure — "how to do X in this kind of project" — not a narration of this one task. Only propose a skill when the conversation demonstrates a non-obvious, repeatable procedure a future agent would benefit from. Prefer a general, umbrella skill covering a class of tasks over a narrow one-off, and write it to apply the next time a similar task arises.

Do NOT propose a skill for any of these:
- environment or dependency failures and their workarounds (transient, machine-specific)
- negative assertions about tools ("tool X doesn't work", "flag Y is unsupported")
- errors that were already resolved during this conversation (transient)
- a one-off task narrative with no reusable procedure
- anything already obvious from reading the project's source or its standard docs

If nothing in the conversation deserves a saved skill, respond with exactly NO_SKILL.

Otherwise output a complete SKILL.md and nothing else — no preamble, no code fence:
---
name: <lowercase-hyphenated-identifier>
description: <what the skill does and WHEN to use it, one or two sentences>
---
<the skill body: concise, imperative instructions the agent will follow next time>`

// askForSkill queries the utility model directly, outside conversation
// middleware, and returns its rendered SKILL.md — or "" when it declines.
func (m *SkillMiner) askForSkill(ctx context.Context, messages []chat.Message) (string, error) {
	transcript := renderTranscript(messages, uncappedToolResults)
	text, err := askDirect(ctx, m.resolveClient(ctx), skillMinerPrompt, transcript)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(text)
	if strings.EqualFold(trimmed, "NO_SKILL") {
		return "", nil
	}
	return unfence(trimmed), nil
}

func (m *SkillMiner) resolveClient(ctx context.Context) *chatclient.Client {
	if m.client == nil {
		return nil
	}
	return m.client(ctx)
}

// unfence strips a single wrapping Markdown code fence when the model wrapped
// its SKILL.md in one despite the instruction not to, so a compliant document
// inside a fence still parses.
func unfence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	}
	s = strings.TrimRight(s, "\n")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
