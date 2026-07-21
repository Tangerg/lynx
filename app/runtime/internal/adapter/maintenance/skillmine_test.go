package maintenance

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type fakeSkillSource struct {
	skills map[string]*skillspec.Skill
}

func (f fakeSkillSource) Load(_ context.Context, name string) (*skillspec.Skill, error) {
	if s, ok := f.skills[name]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("load %q: %w", name, fs.ErrNotExist)
}

const sampleSkillMD = `---
name: run-project-tests
description: How to run the test suite for this Go module. Use when asked to run or verify tests.
---
Run ` + "`go test ./...`" + ` from the module root.`

type fakeDraftStore struct {
	enabled bool
	saved   []skills.Draft
}

func (s *fakeDraftStore) Enabled() bool { return s.enabled }

func (s *fakeDraftStore) SaveDraft(_ context.Context, draft skills.Draft) (skills.DraftHandle, error) {
	s.saved = append(s.saved, draft)
	return skills.DraftHandle{}, nil
}

func minerFixture(t *testing.T, reply string, config MinerConfig) (*SkillMiner, *fakeDraftStore, *textStubModel) {
	t.Helper()
	messages := history.NewInMemoryStore()
	if err := messages.Write(t.Context(), "ses_1",
		chat.NewUserMessage(chat.NewTextPart("add a test target")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
		chat.NewUserMessage(chat.NewTextPart("run the tests")),
		chat.NewAssistantMessage(chat.NewTextPart("passing")),
	); err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel(reply)
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDraftStore{enabled: true}
	miner := NewSkillMiner(messages, store, nil, constClient(client), config)
	return miner, store, model
}

// minerRevisionFixture drives the 2-phase refinement path: a scripted model
// returns phase-one then (optionally) phase-two replies, and source supplies the
// real current skill bodies for the read-before-write guard.
func minerRevisionFixture(t *testing.T, source skillSource, replies ...scriptedReply) (*SkillMiner, *fakeDraftStore) {
	t.Helper()
	messages := history.NewInMemoryStore()
	if err := messages.Write(t.Context(), "ses_1",
		chat.NewUserMessage(chat.NewTextPart("that skill's command is wrong")),
		chat.NewAssistantMessage(chat.NewTextPart("fixing")),
		chat.NewUserMessage(chat.NewTextPart("yes use the new one")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
	); err != nil {
		t.Fatal(err)
	}
	client, err := chatclient.New(&scriptedModel{replies: replies})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDraftStore{enabled: true}
	miner := NewSkillMiner(messages, store, source, constClient(client), MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	return miner, store
}

func TestSkillMinerBelowComplexityThresholdDoesNotMine(t *testing.T) {
	miner, store, model := minerFixture(t, sampleSkillMD, MinerConfig{ComplexityThreshold: 5, Cadence: 1})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 4); err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 {
		t.Fatalf("below-threshold turn called the model %d times", model.calls)
	}
	if len(store.saved) != 0 {
		t.Fatalf("below-threshold turn saved %d drafts", len(store.saved))
	}
}

func TestSkillMinerCadenceGatesMining(t *testing.T) {
	miner, store, model := minerFixture(t, sampleSkillMD, MinerConfig{ComplexityThreshold: 2, Cadence: 2})
	// A routine turn must not advance the cadence counter.
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 1); err != nil {
		t.Fatal(err)
	}
	// First complex turn: due counter reaches 1 of 2 — no mine yet.
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 5); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("mined before the cadence was due: %d drafts", len(store.saved))
	}
	// Second complex turn: cadence is due — mine once.
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 5); err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 {
		t.Fatalf("expected one mining call on the cadence turn, got %d", model.calls)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one saved draft on the cadence turn, got %d", len(store.saved))
	}
}

func TestSkillMinerStampsProvenanceOnSavedDraft(t *testing.T) {
	miner, store, _ := minerFixture(t, sampleSkillMD, MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one saved draft, got %d", len(store.saved))
	}
	draft := store.saved[0]
	if draft.Name != "run-project-tests" {
		t.Errorf("draft name = %q", draft.Name)
	}
	if draft.CreatedBy != skills.CreatedByAgent {
		t.Errorf("draft CreatedBy = %q, want %q", draft.CreatedBy, skills.CreatedByAgent)
	}
	if draft.SourceSession != "ses_1" {
		t.Errorf("draft SourceSession = %q, want %q", draft.SourceSession, "ses_1")
	}
	if !strings.Contains(draft.Body, "go test") {
		t.Errorf("draft body missing distilled procedure: %q", draft.Body)
	}
}

func TestSkillMinerNoSkillProducesNoDraft(t *testing.T) {
	miner, store, model := minerFixture(t, "NO_SKILL", MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 {
		t.Fatalf("expected the model to be consulted once, got %d", model.calls)
	}
	if len(store.saved) != 0 {
		t.Fatalf("NO_SKILL still saved %d drafts", len(store.saved))
	}
}

func TestSkillMinerUnparseableReplyIsDroppedNotErrored(t *testing.T) {
	miner, store, _ := minerFixture(t, "here is a skill but no frontmatter block", MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatalf("unparseable reply surfaced an error: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("unparseable reply saved %d drafts", len(store.saved))
	}
}

func TestSkillMinerFencedReplyStillParses(t *testing.T) {
	fenced := "```markdown\n" + sampleSkillMD + "\n```"
	miner, store, _ := minerFixture(t, fenced, MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("fenced SKILL.md did not yield a draft: %d saved", len(store.saved))
	}
	if store.saved[0].Name != "run-project-tests" {
		t.Errorf("fenced draft name = %q", store.saved[0].Name)
	}
}

func TestSkillMinerRevisionLoadsRealBodyAndMarksRevises(t *testing.T) {
	source := fakeSkillSource{skills: map[string]*skillspec.Skill{
		"run-tests": {
			Frontmatter: skillspec.Frontmatter{Name: "run-tests", Description: "old"},
			Body:        "old body: use make test",
		},
	}}
	corrected := "---\nname: run-tests\ndescription: Run the suite. Use when asked to run tests.\n---\nUse `go test ./...`."
	miner, store := minerRevisionFixture(t, source,
		scriptedReply{text: "REVISE: run-tests"},
		scriptedReply{text: corrected},
	)
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one revision draft, got %d", len(store.saved))
	}
	d := store.saved[0]
	if d.Name != "run-tests" {
		t.Errorf("revision name = %q, want run-tests (a revision must not rename)", d.Name)
	}
	if !d.Revises {
		t.Error("revision draft not marked Revises")
	}
	if d.CreatedBy != skills.CreatedByAgent {
		t.Errorf("revision CreatedBy = %q", d.CreatedBy)
	}
	if !strings.Contains(d.Body, "go test") {
		t.Errorf("revision body did not incorporate the correction: %q", d.Body)
	}
}

func TestSkillMinerRevisionUnknownSkillSkipsWithoutPhaseTwo(t *testing.T) {
	// Only a phase-one reply is scripted: if the miner tried a phase-two call for
	// an unloadable skill, the scripted model would be exhausted and error.
	miner, store := minerRevisionFixture(t, fakeSkillSource{}, scriptedReply{text: "REVISE: ghost"})
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatalf("unknown revision target should skip, got %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("revised a non-existent skill: %d drafts", len(store.saved))
	}
}

func TestSkillMinerDisabledStoreNoOps(t *testing.T) {
	miner, store, model := minerFixture(t, sampleSkillMD, MinerConfig{ComplexityThreshold: 1, Cadence: 1})
	store.enabled = false
	if err := miner.MaybeMine(t.Context(), "ses_1", "/repo", 9); err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || len(store.saved) != 0 {
		t.Fatalf("disabled store still mined: calls=%d saved=%d", model.calls, len(store.saved))
	}
}
