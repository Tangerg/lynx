package runtimeembedded

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/skills"
)

const skillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type skillBindingStub struct {
	t       *testing.T
	actions []string
}

func (stub *skillBindingStub) ListDiscoveredSkills(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.Skill], error) {
	stub.assertCall(request.Workspace.Path, options.RequestMeta)
	return protocol.NewPage([]protocol.Skill{{Name: "release-checks", Description: "Release safely", Scope: protocol.SkillScopeProject}}), nil
}

func (stub *skillBindingStub) ListManagedSkills(_ context.Context, options embedded.CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	stub.assertMeta(options.RequestMeta)
	return protocol.NewPage([]protocol.ManagedSkill{{Name: "review", Description: "Review code", Lifecycle: protocol.SkillLifecycleArchived}}), nil
}

func (stub *skillBindingStub) ListSkillProposals(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	stub.assertCall(request.Workspace.Path, options.RequestMeta)
	return protocol.NewPage([]protocol.SkillProposal{{
		Name: "release-checks", Revision: skillRevision, Scope: protocol.SkillScopeUser,
		Description: "Release safely", Instructions: "Run every release gate.",
		Origin: protocol.SkillProposalOriginRequested, SourceSession: "ses_1",
	}}), nil
}

func (stub *skillBindingStub) ArchiveSkill(_ context.Context, request protocol.SkillNameRequest, options embedded.CommandOptions) error {
	stub.assertCommand("archive:"+request.Name, options)
	return nil
}

func (stub *skillBindingStub) RestoreSkill(_ context.Context, request protocol.SkillNameRequest, options embedded.CommandOptions) error {
	stub.assertCommand("restore:"+request.Name, options)
	return nil
}

func (stub *skillBindingStub) ApproveSkillProposal(_ context.Context, request protocol.SkillProposalRef, options embedded.CommandOptions) error {
	stub.assertReference("approve", request, options)
	return nil
}

func (stub *skillBindingStub) RejectSkillProposal(_ context.Context, request protocol.SkillProposalRef, options embedded.CommandOptions) error {
	stub.assertReference("reject", request, options)
	return nil
}

func (stub *skillBindingStub) assertCall(workspace string, meta protocol.RequestMeta) {
	stub.t.Helper()
	if workspace != "/workspace" {
		stub.t.Fatalf("skill call = workspace %q, meta %+v", workspace, meta)
	}
	stub.assertMeta(meta)
}

func (stub *skillBindingStub) assertMeta(meta protocol.RequestMeta) {
	stub.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		stub.t.Fatalf("skill call meta = %+v", meta)
	}
}

func (stub *skillBindingStub) assertCommand(action string, options embedded.CommandOptions) {
	stub.t.Helper()
	if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
		stub.t.Fatalf("skill command options = %+v", options)
	}
	stub.actions = append(stub.actions, action)
}

func (stub *skillBindingStub) assertReference(action string, request protocol.SkillProposalRef, options embedded.CommandOptions) {
	stub.t.Helper()
	stub.assertCommand(action+":"+string(request.Scope)+"/"+request.Name, options)
	if request.Workspace.Path != "/workspace" || request.Revision != skillRevision {
		stub.t.Fatalf("skill proposal reference = %+v", request)
	}
}

func TestSkillAdapterProjectsCatalogsAndExactMutationReferences(t *testing.T) {
	stub := &skillBindingStub{t: t}
	runtime := &Runtime{skills: stub, meta: requestMeta("test")}
	discovered, err := runtime.Discover(t.Context(), "/workspace")
	if err != nil || len(discovered) != 1 || discovered[0].Key() != "project/release-checks" {
		t.Fatalf("Discover = (%+v, %v)", discovered, err)
	}
	managed, err := runtime.Managed(t.Context())
	if err != nil || len(managed) != 1 || managed[0].Lifecycle != skills.Archived {
		t.Fatalf("Managed = (%+v, %v)", managed, err)
	}
	proposals, err := runtime.Proposals(t.Context(), "/workspace")
	if err != nil || len(proposals) != 1 || proposals[0].Key() != "user/release-checks@0123456789ab" {
		t.Fatalf("Proposals = (%+v, %v)", proposals, err)
	}
	if err := runtime.Archive(t.Context(), "review"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Restore(t.Context(), "review"); err != nil {
		t.Fatal(err)
	}
	reference, err := proposals[0].Reference("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Approve(t.Context(), reference); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reject(t.Context(), reference); err != nil {
		t.Fatal(err)
	}
	want := []string{"archive:review", "restore:review", "approve:user/release-checks", "reject:user/release-checks"}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestServicesExposeOnlyAdvertisedOptionalFeatures(t *testing.T) {
	runtime := &Runtime{enabledFeatures: map[string]struct{}{protocol.FeatureSkills: {}, protocol.FeatureMCP: {}}}
	services := runtime.services()
	if services.Skills == nil || services.MCP == nil || services.Goals != nil {
		t.Fatalf("services = %+v", services)
	}
}
