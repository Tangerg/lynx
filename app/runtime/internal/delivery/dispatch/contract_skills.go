package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerSkills(r *Registry) {
	Query(r, MethodMeta{
		Name:            "skills.discovered.list",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
		return d.api.ListDiscoveredSkills(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "skills.library.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return d.api.ListManagedSkills(ctx)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.library.archive",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.ArchiveSkill(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.library.restore",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.RestoreSkill(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "skills.proposals.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return d.api.ListSkillProposals(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.proposals.approve",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillProposalRef) error {
		return d.api.ApproveSkillProposal(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.proposals.reject",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillProposalRef) error {
		return d.api.RejectSkillProposal(ctx, in)
	})

}
