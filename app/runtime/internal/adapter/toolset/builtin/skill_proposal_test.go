package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type recordingSubmitter struct {
	cwd      string
	proposal skills.Proposal
	calls    int
}

func (s *recordingSubmitter) SubmitProposal(_ context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	s.calls++
	s.cwd = cwd
	s.proposal = proposal
	return skills.NewProposalRef(proposal.Scope, proposal.Name, []byte(proposal.Instructions)), nil
}

func TestProposalSchemaRejectsInvalidDomainValuesBeforeSubmission(t *testing.T) {
	submitter := &recordingSubmitter{}
	candidate, err := NewProposal(submitter, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "ses_1", CWD: "/repo"})
	tests := map[string]string{
		"noncanonical name": `{"name":"Review_Go_API","description":"Review a Go API before implementation.","instructions":"Review it.","scope":"project"}`,
		"overlong name":     `{"name":"` + strings.Repeat("a", 65) + `","description":"Review a Go API before implementation.","instructions":"Review it.","scope":"project"}`,
		"overlong description": `{"name":"review-go-api","description":"` + strings.Repeat("a", 1025) +
			`","instructions":"Review it.","scope":"project"}`,
		"overlong instructions": `{"name":"review-go-api","description":"Review a Go API before implementation.","instructions":"` +
			strings.Repeat("a", skills.MaxAuthoredSkillDocumentBytes+1) + `","scope":"project"}`,
	}
	for name, arguments := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := candidate.Call(ctx, arguments); err == nil || !strings.Contains(err.Error(), "decode function arguments") {
				t.Fatalf("error = %v, want schema rejection", err)
			}
		})
	}
	if submitter.calls != 0 {
		t.Fatalf("invalid proposals reached submitter %d time(s)", submitter.calls)
	}
}

func TestNewNilSubmitterOmitsTool(t *testing.T) {
	candidate, err := NewProposal(nil, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	if candidate != nil {
		t.Fatal("New(nil) returned a tool")
	}
}

func TestDefinitionUsesOnePreciseProposalVocabulary(t *testing.T) {
	candidate, err := NewProposal(&recordingSubmitter{}, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	definition := candidate.Definition()
	if definition.Name != "propose_skill" {
		t.Fatalf("name = %q", definition.Name)
	}
	schema := string(definition.InputSchema)
	for _, required := range []string{`"name"`, `"description"`, `"instructions"`, `"scope"`, `"project"`, `"user"`} {
		if !strings.Contains(schema, required) {
			t.Errorf("schema missing %s: %s", required, schema)
		}
	}
	for _, forbidden := range []string{`"cwd"`, `"session_id"`, `"origin"`, `"publish"`, `"approve"`} {
		if strings.Contains(schema, forbidden) {
			t.Errorf("schema exposes %s: %s", forbidden, schema)
		}
	}
}

func TestCallStampsHostScopeAndReturnsPendingReference(t *testing.T) {
	submitter := &recordingSubmitter{}
	candidate, err := NewProposal(submitter, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{
		SessionID: "ses_1", CWD: "/sandbox", WorkspaceCWD: "/repo", Isolated: true,
	})
	out, err := candidate.Call(ctx, `{
		"name":"review-go-api",
		"description":"Review a Go API before implementation. Use when a design changes exported behavior.",
		"instructions":"Read the design, inspect consumers, and report compatibility risks.",
		"scope":"project"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if submitter.cwd != "/repo" {
		t.Fatalf("cwd = %q", submitter.cwd)
	}
	if submitter.proposal.Scope != skills.ScopeProject || submitter.proposal.Origin != skills.ProposalOriginRequested || submitter.proposal.SourceSession != "ses_1" {
		t.Fatalf("proposal provenance = %+v", submitter.proposal)
	}
	var got proposalResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode result %q: %v", out, err)
	}
	if got.Status != "pending_review" || got.Name != "review-go-api" || got.Scope != "project" || got.Revision == "" {
		t.Fatalf("result = %+v", got)
	}
}

func TestCallRequiresSessionAndValidScope(t *testing.T) {
	candidate, err := NewProposal(&recordingSubmitter{}, "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	validArguments := `{"name":"review-go-api","description":"Review a Go API before implementation.","instructions":"Review it.","scope":"project"}`
	if _, err := candidate.Call(context.Background(), validArguments); err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("no-session error = %v", err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "ses_1", CWD: "/repo"})
	invalidArguments := `{"name":"review-go-api","description":"Review a Go API before implementation.","instructions":"Review it.","scope":"team"}`
	if _, err := candidate.Call(ctx, invalidArguments); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("invalid-scope error = %v", err)
	}
}
