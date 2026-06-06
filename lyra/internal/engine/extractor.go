package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	lyramem "github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// extractor folds long-term knowledge out of a session and appends
// it to the project-scope LYRA.md. The intent is "things this
// conversation taught us that the next session should already know
// without re-deriving" — file structure invariants, user
// preferences stated mid-turn, repeated commands the user prefers,
// etc.
//
// Runs after a compaction sweep so the LLM sees a manageable slice
// of recent history. Failure is non-fatal — the conversation has
// already been compacted; we'd rather skip the extraction than
// undo that.
type extractor struct {
	store   memory.Store
	memSvc  lyramem.Service
	client  *chat.Client
	minMsgs int
}

// ExtractionResult reports what a single [Engine.MaybeExtract] pass
// wrote to long-term memory. Extracted is false (Facts empty) when
// nothing was mined — no memory service, conversation too short, or
// the LLM judged nothing worth keeping. Facts is the markdown the
// pass appended to LYRA.md, so callers can surface an observable
// "saved N notes to memory" event.
type ExtractionResult struct {
	Extracted bool
	Facts     string
}

func newExtractor(store memory.Store, memSvc lyramem.Service, client *chat.Client) *extractor {
	return &extractor{
		store:   store,
		memSvc:  memSvc,
		client:  client,
		minMsgs: 4, // at least 2 exchanges before extracting
	}
}

// maybeExtract reads the post-compaction history, asks the LLM
// what's worth keeping long-term, and appends the result to
// LYRA.md's project scope. Returns nil when the engine has no
// memory service (LYRA.md disabled), or when the conversation is
// still too short to be worth mining.
func (e *extractor) maybeExtract(ctx context.Context, sessionID string) (ExtractionResult, error) {
	if e == nil || sessionID == "" {
		return ExtractionResult{}, nil
	}
	msgs, err := e.store.Read(ctx, sessionID)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("extractor: read: %w", err)
	}
	if len(msgs) < e.minMsgs {
		return ExtractionResult{}, nil
	}

	facts, err := e.askForFacts(ctx, msgs)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("extractor: ask: %w", err)
	}
	if facts == "" {
		return ExtractionResult{}, nil
	}

	existing, err := e.memSvc.Get(ctx, lyramem.ScopeProject)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("extractor: read memory: %w", err)
	}
	updated := mergeMemory(existing, facts)
	if err := e.memSvc.Update(ctx, lyramem.ScopeProject, updated); err != nil {
		return ExtractionResult{}, err
	}
	return ExtractionResult{Extracted: true, Facts: facts}, nil
}

// askForFacts queries the LLM directly (no middleware → no
// recursion into the chat-memory layer). Returns "" when the LLM
// explicitly says nothing is worth recording.
func (e *extractor) askForFacts(ctx context.Context, msgs []chat.Message) (string, error) {
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

	text, err := askDirect(ctx, e.client, prompt, transcript)
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
// Idempotency: if existing already ends with the same fact body,
// no append happens (a small guard against retry loops appending
// duplicates).
func mergeMemory(existing, facts string) string {
	const header = "## Lyra-extracted facts"
	facts = strings.TrimSpace(facts)
	if strings.Contains(existing, facts) {
		return existing
	}
	if existing == "" {
		return header + "\n\n" + facts + "\n"
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if !strings.Contains(existing, header) {
		return existing + "\n" + header + "\n\n" + facts + "\n"
	}
	return existing + facts + "\n"
}
