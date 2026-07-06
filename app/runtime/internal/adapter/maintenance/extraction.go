package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// Extractor folds long-term knowledge out of a session and appends
// it to the project-scope LYRA.md. The intent is "things this
// conversation taught us that the next session should already know
// without re-deriving" — file structure invariants, user
// preferences stated mid-turn, repeated commands the user prefers,
// etc.
//
// Run after a compaction sweep so the LLM sees a manageable slice
// of recent history. Failure is non-fatal — the conversation has
// already been compacted; skipping the extraction is preferable
// to undoing a successful compaction.
type Extractor struct {
	store   history.Store
	memSvc  knowledge.Service
	client  ClientFunc
	minMsgs int
}

// NewExtractor builds an Extractor over the chat history store, the
// long-term LYRA.md service, and a per-call chat-client resolver.
func NewExtractor(store history.Store, memSvc knowledge.Service, client ClientFunc) *Extractor {
	return &Extractor{
		store:   store,
		memSvc:  memSvc,
		client:  client,
		minMsgs: 4, // at least 2 exchanges before extracting
	}
}

// MaybeExtract reads the post-compaction history, asks the LLM
// what's worth keeping long-term, and appends the result to the
// project-scope LYRA.md of cwd — the session's working directory, so
// facts land in the project the conversation was about (empty cwd
// falls back to the memory service's default dir). Returns the zero
// result on a nil receiver (LYRA.md disabled) or when the conversation
// is still too short to be worth mining.
func (e *Extractor) MaybeExtract(ctx context.Context, sessionID, cwd string) (kernel.ExtractionResult, error) {
	if e == nil || sessionID == "" {
		return kernel.ExtractionResult{}, nil
	}
	msgs, err := e.store.Read(ctx, sessionID)
	if err != nil {
		return kernel.ExtractionResult{}, fmt.Errorf("extractor: read: %w", err)
	}
	if len(msgs) < e.minMsgs {
		return kernel.ExtractionResult{}, nil
	}

	facts, err := e.askForFacts(ctx, msgs)
	if err != nil {
		return kernel.ExtractionResult{}, fmt.Errorf("extractor: ask: %w", err)
	}
	if facts == "" {
		return kernel.ExtractionResult{}, nil
	}

	existing, err := e.memSvc.Get(ctx, knowledge.ScopeProject, cwd)
	if err != nil {
		return kernel.ExtractionResult{}, fmt.Errorf("extractor: read memory: %w", err)
	}
	updated := mergeMemory(existing, facts)
	if err := e.memSvc.Update(ctx, knowledge.ScopeProject, cwd, updated); err != nil {
		return kernel.ExtractionResult{}, err
	}
	return kernel.ExtractionResult{Extracted: true, Facts: facts}, nil
}

// askForFacts queries the LLM directly (no middleware → no
// recursion into the chat history layer). Returns "" when the LLM
// explicitly says nothing is worth recording.
func (e *Extractor) askForFacts(ctx context.Context, msgs []chat.Message) (string, error) {
	transcript := renderTranscript(msgs)
	const prompt = `You are mining a coding-agent conversation for facts worth
adding to a project's persistent memory file (LYRA.md). Output
short markdown bullets — each one a stand-alone, self-explanatory
fact the next session should already know.

Include things like: file conventions, build / test commands, the
user's stated preferences, project-specific terminology, recurring
gotchas. Exclude: one-off observations, transient state, anything
visible in the source itself.

If nothing in the conversation is worth permanent recording,
respond with exactly: NO_FACTS

Otherwise output ONLY the bullets, no preamble or trailing text.`

	var client *chat.Client
	if e.client != nil {
		client = e.client(ctx)
	}
	text, err := askDirect(ctx, client, prompt, transcript)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed == "NO_FACTS" {
		return "", nil
	}
	return trimmed, nil
}

// mergeMemory appends new facts to the existing memory under a
// stable "## Lyra-extracted facts" heading so manually edited
// content and auto-extracted content stay visually separated.
// Idempotency is PER-LINE: each bullet already present verbatim is
// dropped, the rest are appended. A whole-blob check (does existing
// contain the new block?) would discard the ENTIRE block whenever a
// single bullet recurred — losing the genuinely-new facts alongside it.
func mergeMemory(existing, facts string) string {
	const header = "## Lyra-extracted facts"
	facts = strings.TrimSpace(facts)
	if facts == "" {
		return existing
	}
	// Exact-line membership of what's already saved (trimmed), so a recurring
	// bullet is skipped while new bullets in the same block survive.
	seen := make(map[string]bool)
	for line := range strings.SplitSeq(existing, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			seen[t] = true
		}
	}
	var fresh []string
	for line := range strings.SplitSeq(facts, "\n") {
		if t := strings.TrimSpace(line); t != "" && seen[t] {
			continue
		}
		fresh = append(fresh, line)
	}
	merged := strings.TrimSpace(strings.Join(fresh, "\n"))
	if merged == "" {
		return existing // every new bullet was already present
	}
	if existing == "" {
		return header + "\n\n" + merged + "\n"
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if !strings.Contains(existing, header) {
		return existing + "\n" + header + "\n\n" + merged + "\n"
	}
	return existing + merged + "\n"
}
