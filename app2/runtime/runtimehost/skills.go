package runtimehost

import (
	"context"

	lyraskills "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app2/runtime/agenttools"
	"github.com/Tangerg/lynx/app2/runtime/capabilityflow"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimeevents"
)

// runtimeSkillGateway adapts the Agent's narrow Skill capability to the
// Runtime owner. Agent-authored provenance is intentionally fixed here rather
// than accepted through model-visible tool arguments.
type runtimeSkillGateway struct {
	capabilities *capabilityflow.Service
	events       *runtimeevents.Bus
}

func (gateway runtimeSkillGateway) Source(
	ctx context.Context,
	workspace string,
) (lyraskills.ResourceSource, error) {
	return gateway.capabilities.SkillSource(ctx, workspace)
}

func (gateway runtimeSkillGateway) ProposeSkill(
	ctx context.Context,
	draft agenttools.SkillProposalDraft,
) (*protocol.SkillProposal, error) {
	proposal, err := gateway.capabilities.ProposeSkill(ctx, capabilityflow.SkillProposalDraft{
		Workspace:     protocol.WorkspaceRef{Path: draft.Workspace},
		SourceSession: draft.SourceSession,
		Name:          draft.Name,
		Description:   draft.Description,
		Instructions:  draft.Instructions,
		Scope:         draft.Scope,
		Origin:        protocol.SkillProposalOriginRequested,
	})
	if err == nil {
		gateway.events.Publish(protocol.RuntimeEvent{
			Type:  protocol.RuntimeSkillsChanged,
			Names: []string{proposal.Name},
		})
	}
	return proposal, err
}
