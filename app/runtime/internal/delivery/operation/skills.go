package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	SkillsDiscoveredList   Name = "skills.discovered.list"
	SkillsLibraryList      Name = "skills.library.list"
	SkillsLibraryArchive   Name = "skills.library.archive"
	SkillsLibraryRestore   Name = "skills.library.restore"
	SkillsProposalsList    Name = "skills.proposals.list"
	SkillsProposalsApprove Name = "skills.proposals.approve"
	SkillsProposalsReject  Name = "skills.proposals.reject"
)

func registerSkills(registry *Registry) {
	registry.Query(MethodMeta{
		Name:            SkillsDiscoveredList,
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
		return service.ListDiscoveredSkills(ctx, request)
	})

	registry.Query(MethodMeta{
		Name:            SkillsLibraryList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	registry.CommandAck(MethodMeta{
		Name:            SkillsLibraryArchive,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ArchiveSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.ArchiveSkill(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name:            SkillsLibraryRestore,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RestoreSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.RestoreSkill(ctx, request)
	})

	registry.Query(MethodMeta{
		Name:            SkillsProposalsList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return service.ListSkillProposals(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name:            SkillsProposalsApprove,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ApproveSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.ApproveSkillProposal(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name:            SkillsProposalsReject,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RejectSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.RejectSkillProposal(ctx, request)
	})

}
