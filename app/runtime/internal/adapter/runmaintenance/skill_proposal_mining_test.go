package runmaintenance

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/chathistory/inmemory"
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

type fakeProposalSubmitter struct {
	proposals []skills.Proposal
	cwds      []string
}

func (s *fakeProposalSubmitter) SubmitProposal(_ context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	s.cwds = append(s.cwds, cwd)
	s.proposals = append(s.proposals, proposal)
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), nil
}

func skillProposalMinerFixture(t *testing.T, reply string, config SkillMiningConfig) (*SkillProposalMiner, *fakeProposalSubmitter, *textStubModel) {
	t.Helper()
	messages := inmemory.New()
	if err := messages.Write(t.Context(), "ses_1",
		chat.NewUserMessage(chat.NewTextPart("add a test target")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
		chat.NewUserMessage(chat.NewTextPart("run the tests")),
		chat.NewAssistantMessage(chat.NewTextPart("passing")),
	); err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel(reply)
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	proposals := &fakeProposalSubmitter{}
	skillMiner := NewSkillProposalMiner(messages, proposals, nil, constClient(client), config)
	return skillMiner, proposals, model
}

// skillRevisionMinerFixture drives the 2-phase refinement path: a scripted model
// returns phase-one then (optionally) phase-two replies, and source supplies the
// real current skill bodies for the read-before-write guard.
func skillRevisionMinerFixture(t *testing.T, source skillSource, replies ...scriptedReply) (*SkillProposalMiner, *fakeProposalSubmitter) {
	t.Helper()
	messages := inmemory.New()
	if err := messages.Write(t.Context(), "ses_1",
		chat.NewUserMessage(chat.NewTextPart("that skill's command is wrong")),
		chat.NewAssistantMessage(chat.NewTextPart("fixing")),
		chat.NewUserMessage(chat.NewTextPart("yes use the new one")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
	); err != nil {
		t.Fatal(err)
	}
	client, err := chatclient.New(&scriptedModel{replies: replies}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	proposals := &fakeProposalSubmitter{}
	skillMiner := NewSkillProposalMiner(messages, proposals, source, constClient(client), SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	return skillMiner, proposals
}

func TestSkillMinerBelowComplexityThresholdDoesNotMine(t *testing.T) {
	skillMiner, proposals, model := skillProposalMinerFixture(t, sampleSkillMD, SkillMiningConfig{ComplexityThreshold: 5, Cadence: 1})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 4); err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 {
		t.Fatalf("below-threshold Run called the model %d times", model.calls)
	}
	if len(proposals.proposals) != 0 {
		t.Fatalf("below-threshold Run submitted %d proposals", len(proposals.proposals))
	}
}

func TestSkillMinerCadenceGatesMining(t *testing.T) {
	skillMiner, proposals, model := skillProposalMinerFixture(t, sampleSkillMD, SkillMiningConfig{ComplexityThreshold: 2, Cadence: 2})
	// A routine Run must not advance the cadence counter.
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 1); err != nil {
		t.Fatal(err)
	}
	// First complex Run: due counter reaches 1 of 2 — no mine yet.
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 5); err != nil {
		t.Fatal(err)
	}
	if len(proposals.proposals) != 0 {
		t.Fatalf("mined before the cadence was due: %d proposals", len(proposals.proposals))
	}
	// Second complex Run: cadence is due — mine once.
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 5); err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 {
		t.Fatalf("expected one mining call on the cadence Run, got %d", model.calls)
	}
	if len(proposals.proposals) != 1 {
		t.Fatalf("expected one proposal on the cadence Run, got %d", len(proposals.proposals))
	}
}

func TestSkillMinerSubmitsUserProposalWithMinedProvenance(t *testing.T) {
	skillMiner, proposals, model := skillProposalMinerFixture(t, sampleSkillMD, SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(proposals.proposals) != 1 {
		t.Fatalf("expected one proposal, got %d", len(proposals.proposals))
	}
	if len(model.requests) != 1 || model.requests[0].Options.MaxTokens == nil || *model.requests[0].Options.MaxTokens != skillMiningOutputTokens {
		t.Fatalf("skill mining MaxTokens = %#v, want %d", model.requests, skillMiningOutputTokens)
	}
	proposal := proposals.proposals[0]
	if proposal.Name != "run-project-tests" {
		t.Errorf("proposal name = %q", proposal.Name)
	}
	if proposal.Scope != skills.ScopeUser {
		t.Errorf("proposal Scope = %q, want %q", proposal.Scope, skills.ScopeUser)
	}
	if proposal.Origin != skills.ProposalOriginMined {
		t.Errorf("proposal Origin = %q, want %q", proposal.Origin, skills.ProposalOriginMined)
	}
	if proposal.SourceSession != "ses_1" {
		t.Errorf("proposal SourceSession = %q, want %q", proposal.SourceSession, "ses_1")
	}
	if !strings.Contains(proposal.Instructions, "go test") {
		t.Errorf("proposal instructions missing distilled procedure: %q", proposal.Instructions)
	}
	if len(proposals.cwds) != 1 || proposals.cwds[0] != "/repo" {
		t.Errorf("proposal cwd = %+v", proposals.cwds)
	}
}

func TestSkillMinerNoSkillProducesNoProposal(t *testing.T) {
	skillMiner, proposals, model := skillProposalMinerFixture(t, "NO_SKILL", SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 {
		t.Fatalf("expected the model to be consulted once, got %d", model.calls)
	}
	if len(proposals.proposals) != 0 {
		t.Fatalf("NO_SKILL still submitted %d proposals", len(proposals.proposals))
	}
}

func TestSkillMinerUnparseableReplyIsDroppedNotErrored(t *testing.T) {
	skillMiner, proposals, _ := skillProposalMinerFixture(t, "here is a skill but no frontmatter block", SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatalf("unparseable reply surfaced an error: %v", err)
	}
	if len(proposals.proposals) != 0 {
		t.Fatalf("unparseable reply submitted %d proposals", len(proposals.proposals))
	}
}

func TestSkillMinerFencedReplyStillParses(t *testing.T) {
	fenced := "```markdown\n" + sampleSkillMD + "\n```"
	skillMiner, proposals, _ := skillProposalMinerFixture(t, fenced, SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(proposals.proposals) != 1 {
		t.Fatalf("fenced SKILL.md did not yield a proposal: %d submitted", len(proposals.proposals))
	}
	if proposals.proposals[0].Name != "run-project-tests" {
		t.Errorf("fenced proposal name = %q", proposals.proposals[0].Name)
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
	skillMiner, proposals := skillRevisionMinerFixture(t, source,
		scriptedReply{text: "REVISE: run-tests"},
		scriptedReply{text: corrected},
	)
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatal(err)
	}
	if len(proposals.proposals) != 1 {
		t.Fatalf("expected one revision proposal, got %d", len(proposals.proposals))
	}
	d := proposals.proposals[0]
	if d.Name != "run-tests" {
		t.Errorf("revision name = %q, want run-tests (a revision must not rename)", d.Name)
	}
	if !d.Revises {
		t.Error("revision proposal not marked Revises")
	}
	if d.Origin != skills.ProposalOriginMined {
		t.Errorf("revision Origin = %q", d.Origin)
	}
	if !strings.Contains(d.Instructions, "go test") {
		t.Errorf("revision instructions did not incorporate the correction: %q", d.Instructions)
	}
}

func TestSkillMinerRevisionUnknownSkillSkipsWithoutPhaseTwo(t *testing.T) {
	// Only a phase-one reply is scripted: if the skillMiner tried a phase-two call for
	// an unloadable skill, the scripted model would be exhausted and error.
	skillMiner, proposals := skillRevisionMinerFixture(t, fakeSkillSource{}, scriptedReply{text: "REVISE: ghost"})
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 3); err != nil {
		t.Fatalf("unknown revision target should skip, got %v", err)
	}
	if len(proposals.proposals) != 0 {
		t.Fatalf("revised a non-existent Skill: %d proposals", len(proposals.proposals))
	}
}

func TestSkillMinerWithoutProposalSubmitterNoOps(t *testing.T) {
	skillMiner, proposals, model := skillProposalMinerFixture(t, sampleSkillMD, SkillMiningConfig{ComplexityThreshold: 1, Cadence: 1})
	skillMiner.proposals = nil
	if err := skillMiner.MineIfDue(t.Context(), "ses_1", "/repo", 9); err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || len(proposals.proposals) != 0 {
		t.Fatalf("missing submitter still mined: calls=%d proposals=%d", model.calls, len(proposals.proposals))
	}
}
