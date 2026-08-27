package agentexec

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	apphooks "github.com/Tangerg/scope/app/runtime/internal/application/hooks"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
	domainhooks "github.com/Tangerg/scope/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/metadata"
)

func TestSystemPromptProvenanceMatchesVisibleComposition(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	document := filepath.Join(cwd, "AGENTS.md")
	if err := os.WriteFile(document, []byte("agent document"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalDocument, err := filepath.EvalSymlinks(document)
	if err != nil {
		t.Fatal(err)
	}
	knowledge := &stubKnowledgeStore{home: "user rule", cwd: "workspace rule"}
	memory := provenanceMemoryReader{items: []agentmemory.Item{{
		ID: "memory:pinned", Content: "remember this", Pinned: true,
	}}}
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Knowledge:   knowledge,
		AgentMemory: memory,
		Plan:        provenancePlanReader{},
	})
	message, err := composer.composeSystemMessage(t.Context(), "session:one", cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}

	provenance := decodeContextProvenance(t, message.Metadata)
	wantKinds := []contextSourceKind{
		contextSourceBasePrompt,
		contextSourceUserKnowledge,
		contextSourcePinnedMemory,
		contextSourceProjectKnowledge,
		contextSourceAgentDocument,
		contextSourceSessionPlan,
	}
	gotKinds := make([]contextSourceKind, len(provenance.Sources))
	for index, source := range provenance.Sources {
		gotKinds[index] = source.Kind
	}
	if !slices.Equal(gotKinds, wantKinds) {
		t.Fatalf("source kinds=%v, want %v", gotKinds, wantKinds)
	}
	if provenance.Sources[2].Reference != "memory:pinned" ||
		provenance.Sources[2].Purpose != contextPurposeData ||
		provenance.Sources[4].Reference != canonicalDocument ||
		provenance.Sources[5].Reference != "session:one" {
		t.Fatalf("provenance=%+v", provenance)
	}
}

func TestPromptCompositionRejectsSourcePurposeMismatch(t *testing.T) {
	var prompt promptComposition
	prompt.append(basePrompt, contextSource{
		Kind: contextSourceBasePrompt, Purpose: contextPurposeData,
	})
	if _, err := prompt.systemMessage(); err == nil {
		t.Fatal("source kind and purpose mismatch must be rejected")
	}
}

func TestWorkingContextAttributesHookAndRecalledMemoryInPlace(t *testing.T) {
	cwd := t.TempDir()
	hooks := []domainhooks.Hook{
		{Event: domainhooks.SessionStart, Inject: "session context"},
		{Event: domainhooks.UserPromptSubmit, Inject: "turn context"},
	}
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Hooks: provenanceHookResolver{bound: apphooks.NewBound(hooks, apphooks.NewRunner(nil, nil))},
		AgentMemorySearch: &fakeAgentMemorySearcher{
			items: []agentmemory.Item{{ID: "memory:recalled", Content: "recalled fact"}},
		},
	})
	messages, err := composer.ComposeWorkingContext(t.Context(), runs.WorkingContextInput{
		SessionID:  "session:one",
		CWD:        cwd,
		PromptText: "question",
		Seed: []corechat.Message{
			corechat.NewUserMessage(corechat.NewTextPart("question")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages=%d, want system + recall + user", len(messages))
	}
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			t.Fatalf("message[%d]: %v", index, err)
		}
	}

	system := decodeContextProvenance(t, messages[0].Metadata)
	if len(system.Sources) != 1 || system.Sources[0].Kind != contextSourceBasePrompt {
		t.Fatalf("system provenance=%+v", system)
	}
	recalled := decodeContextProvenance(t, messages[1].Metadata)
	if len(recalled.Sources) != 1 || recalled.Sources[0].Kind != contextSourceRecalledMemory ||
		recalled.Sources[0].Reference != "memory:recalled" ||
		recalled.Sources[0].Purpose != contextPurposeData {
		t.Fatalf("recall provenance=%+v", recalled)
	}
	hook := decodeContextProvenance(t, messages[2].Parts[0].Metadata)
	if len(hook.Sources) != 2 ||
		hook.Sources[0].Reference != string(domainhooks.SessionStart) ||
		hook.Sources[1].Reference != string(domainhooks.UserPromptSubmit) {
		t.Fatalf("hook provenance=%+v", hook)
	}
	if len(messages[2].Parts) != 2 || messages[2].Parts[1].Text != "question" {
		t.Fatalf("hook injection changed user part ordering: %+v", messages[2].Parts)
	}
}

func decodeContextProvenance(t *testing.T, values metadata.Map) contextProvenance {
	t.Helper()
	provenance, found, err := values.Decode[contextProvenance](contextProvenanceMetadataKey)
	if err != nil || !found {
		t.Fatalf("decode context provenance found=%t error=%v", found, err)
	}
	if provenance.SchemaVersion != contextProvenanceSchemaVersion || len(provenance.Sources) == 0 {
		t.Fatalf("context provenance=%+v", provenance)
	}
	return provenance
}

type provenanceMemoryReader struct {
	items []agentmemory.Item
}

func (p provenanceMemoryReader) Items(
	_ context.Context,
	scope agentmemory.Scope,
	_ string,
) ([]agentmemory.Item, error) {
	if scope != agentmemory.ScopeProject {
		return nil, nil
	}
	return slices.Clone(p.items), nil
}

type provenancePlanReader struct{}

func (provenancePlanReader) List(context.Context, string) ([]plan.Step, error) {
	return []plan.Step{{Description: "verify provenance", Status: plan.StatusInProgress}}, nil
}

type provenanceHookResolver struct {
	bound *apphooks.Bound
}

func (p provenanceHookResolver) For(context.Context, string) (*apphooks.Bound, error) {
	return p.bound, nil
}
