package agenttools

import (
	"context"
	"fmt"
	"strings"

	lyraskills "github.com/Tangerg/lynx/skills"
	toolcontract "github.com/Tangerg/lynx/tool"
	skilltools "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// SkillProposalDraft contains only values the Agent is allowed to author.
// The Runtime adapter supplies trusted origin and emits the invalidation.
type SkillProposalDraft struct {
	Workspace     string
	SourceSession string
	Name          string
	Description   string
	Instructions  string
	Scope         protocol.SkillScope
}

type SkillGateway interface {
	Source(context.Context, string) (lyraskills.ResourceSource, error)
	ProposeSkill(context.Context, SkillProposalDraft) (*protocol.SkillProposal, error)
}

type proposeSkillRequest struct {
	Name         string              `json:"name" jsonschema:"required,minLength=1,maxLength=64,pattern=^[a-z0-9]+(-[a-z0-9]+)*$" jsonschema_description:"Stable lowercase hyphenated Skill name, such as review-go-api."`
	Description  string              `json:"description" jsonschema:"required,minLength=1,maxLength=1024" jsonschema_description:"What reusable workflow this Skill provides and when to use it."`
	Instructions string `json:"instructions" jsonschema:"required,minLength=1" jsonschema_description:"Complete self-contained future instructions. Exclude transient progress, one-off context, and secrets."`
	Scope        protocol.SkillScope `json:"scope" jsonschema:"required,enum=project,enum=user" jsonschema_description:"project makes it available only in this workspace; user makes it available across this user's workspaces."`
}

type proposeSkillResult struct {
	Status   string              `json:"status"`
	Name     string              `json:"name"`
	Revision string              `json:"revision"`
	Scope    protocol.SkillScope `json:"scope"`
}

func (catalog *Catalog) skillTools(
	ctx context.Context,
	scope agentexec.ToolScope,
) ([]scopedTool, error) {
	source, err := catalog.skillGateway.Source(ctx, scope.Workspace)
	if err != nil {
		return nil, fmt.Errorf("agenttools: resolve Skill source: %w", err)
	}
	progressive, err := skilltools.NewTools(source)
	if err != nil {
		return nil, fmt.Errorf("agenttools: build progressive Skill tools: %w", err)
	}
	values := make([]scopedTool, 0, len(progressive)+1)
	for _, executable := range progressive {
		values = append(values, scopedTool{tool: executable, safety: protocol.SafetyClassSafe})
	}
	if !scope.IsRootRun {
		return values, nil
	}
	propose, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "propose_skill",
			Description: `Propose a reusable Lyra Skill for human review. Call only when the user explicitly asks to save, preserve, or create a reusable workflow. Do not call for one-off facts, ordinary progress, transient fixes, or secrets. This creates a pending proposal; it never activates or executes the Skill.`,
		},
		func(ctx context.Context, request proposeSkillRequest) (proposeSkillResult, error) {
			proposal, err := catalog.skillGateway.ProposeSkill(ctx, SkillProposalDraft{
				Workspace:     scope.Workspace,
				SourceSession: scope.SessionID,
				Name:          strings.TrimSpace(request.Name),
				Description:   strings.TrimSpace(request.Description),
				Instructions:  strings.TrimSpace(request.Instructions),
				Scope:         request.Scope,
			})
			if err != nil {
				return proposeSkillResult{}, err
			}
			return proposeSkillResult{
				Status:   "pending_review",
				Name:     proposal.Name,
				Revision: proposal.Revision,
				Scope:    proposal.Scope,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("agenttools: build propose_skill: %w", err)
	}
	return append(values, scopedTool{tool: propose, safety: protocol.SafetyClassSafe}), nil
}
