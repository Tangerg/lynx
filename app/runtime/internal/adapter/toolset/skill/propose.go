package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const proposalDescription = `Propose a reusable Skill for future work.

Call this only when the user explicitly asks to save, preserve, or create a
reusable workflow. Do not call it merely because a workflow seems useful, and
do not use it for one-off facts, ordinary task progress, transient fixes, or
secrets.

This creates a pending proposal for review. It does not activate, publish, or
run the Skill.`

type proposalArgs struct {
	Name         string `json:"name" jsonschema:"required,minLength=1,maxLength=64,pattern=^[a-z0-9]+(-[a-z0-9]+)*$" jsonschema_description:"Stable lowercase hyphenated Skill identifier of at most 64 characters, such as review-go-api. Describe the workflow, not the current task."`
	Description  string `json:"description" jsonschema:"required,minLength=1,maxLength=1024" jsonschema_description:"One or two sentences stating what reusable workflow the Skill provides and when it should be used."`
	Instructions string `json:"instructions" jsonschema:"required,minLength=1" jsonschema_description:"Complete self-contained instructions an Agent should follow in future work. Exclude one-off progress, transient context, and secrets."`
	Scope        string `json:"scope" jsonschema:"required,enum=project,enum=user" jsonschema_description:"project = available only in the current workspace; user = available across all workspaces for this user."`
}

type proposalResult struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Scope    string `json:"scope"`
}

// ProposalSubmitter is the only Skill-authoring operation this capability consumes.
type ProposalSubmitter interface {
	SubmitSkillProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error)
}

type proposer struct {
	proposals            ProposalSubmitter
	defaultWorkspacePath string
}

// NewProposal builds propose_skill. A nil submitter omits the capability.
func NewProposal(proposals ProposalSubmitter, defaultWorkspacePath string) (toolcontract.Tool, error) {
	if proposals == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[proposalArgs, proposalResult](
		toolcontract.FuncConfig{Name: catalog.ProposeSkill, Description: proposalDescription},
		(&proposer{proposals: proposals, defaultWorkspacePath: defaultWorkspacePath}).run,
	)
}

func (t *proposer) run(ctx context.Context, input proposalArgs) (proposalResult, error) {
	sessionID := strings.TrimSpace(executionctx.SessionID(ctx))
	if sessionID == "" {
		return proposalResult{}, errors.New("propose_skill: no active session")
	}
	cwd := strings.TrimSpace(executionctx.WorkspaceCWD(ctx, t.defaultWorkspacePath))
	if cwd == "" {
		return proposalResult{}, errors.New("propose_skill: no active workspace")
	}
	scope, err := parseScope(input.Scope)
	if err != nil {
		return proposalResult{}, err
	}
	proposal := skills.Proposal{
		Scope:         scope,
		Name:          strings.TrimSpace(input.Name),
		Description:   strings.TrimSpace(input.Description),
		Instructions:  strings.TrimSpace(input.Instructions),
		Origin:        skills.ProposalOriginRequested,
		SourceSession: sessionID,
	}
	if err := proposal.Validate(); err != nil {
		return proposalResult{}, fmt.Errorf("propose_skill: invalid proposal: %w", err)
	}
	ref, err := t.proposals.SubmitSkillProposal(ctx, cwd, proposal)
	if err != nil {
		return proposalResult{}, fmt.Errorf("propose_skill: submit proposal: %w", err)
	}
	return proposalResult{
		Status:   "pending_review",
		Name:     ref.Name,
		Revision: ref.Revision,
		Scope:    string(ref.Scope),
	}, nil
}

func parseScope(value string) (skills.Scope, error) {
	scope := skills.Scope(strings.TrimSpace(value))
	if err := scope.Validate(); err != nil {
		return "", fmt.Errorf("propose_skill: scope must be project or user: %w", err)
	}
	return scope, nil
}
