package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerSkills(registry *Registry) {
	Query(registry, MethodMeta{
		Name:            "skills.discovered.list",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
		return service.ListDiscoveredSkills(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "skills.library.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.library.archive",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.ArchiveSkill(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.library.restore",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.RestoreSkill(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "skills.proposals.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return service.ListSkillProposals(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.proposals.approve",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.ApproveSkillProposal(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "skills.proposals.reject",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(service Service, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.RejectSkillProposal(ctx, request)
	})

}
