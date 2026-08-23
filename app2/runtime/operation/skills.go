package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerSkills(registry *Registry) {
	Query(registry, MethodMeta{
		Name:            "skills.discovered.list",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
		return service.ListDiscoveredSkills(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "skills.library.list",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.library.archive",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ArchiveSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.ArchiveSkill(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.library.restore",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RestoreSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.RestoreSkill(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "skills.proposals.list",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return service.ListSkillProposals(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.proposals.approve",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ApproveSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.ApproveSkillProposal(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.proposals.reject",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RejectSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.RejectSkillProposal(ctx, request)
	})

}
