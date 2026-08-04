// Package proposeskill exposes the root Agent's propose_skill tool over the
// narrow Skill-proposal application use case. It cannot review or activate a
// Skill.
package proposeskill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const description = `Propose a reusable Skill for future work.

Call this only when the user explicitly asks to save, preserve, or create a
reusable workflow. Do not call it merely because a workflow seems useful, and
do not use it for one-off facts, ordinary task progress, transient fixes, or
secrets.

This creates a pending proposal for review. It does not activate, publish, or
run the Skill.`

type args struct {
	Name         string `json:"name" jsonschema:"required,minLength=1" jsonschema_description:"Stable lowercase hyphenated Skill identifier, such as review-go-api. Describe the workflow, not the current task."`
	Description  string `json:"description" jsonschema:"required,minLength=1" jsonschema_description:"One or two sentences stating what reusable workflow the Skill provides and when it should be used."`
	Instructions string `json:"instructions" jsonschema:"required,minLength=1" jsonschema_description:"Complete self-contained instructions an Agent should follow in future work. Exclude one-off progress, transient context, and secrets."`
	Scope        string `json:"scope" jsonschema:"required,enum=project,enum=user" jsonschema_description:"project = available only in the current workspace; user = available across all workspaces for this user."`
}

type result struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Scope    string `json:"scope"`
}

// Submitter is the only Skill-authoring operation this tool consumes.
type Submitter interface {
	SubmitSkillProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error)
}

type tool struct {
	proposals  Submitter
	defaultCwd string
}

// New builds propose_skill. A nil submitter omits the capability.
func New(proposals Submitter, defaultCwd string) (toolcontract.Tool, error) {
	if proposals == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[args, result](
		toolcontract.FuncConfig{Name: "propose_skill", Description: description},
		(&tool{proposals: proposals, defaultCwd: defaultCwd}).run,
	)
}

func (t *tool) run(ctx context.Context, input args) (result, error) {
	sessionID := strings.TrimSpace(executionctx.SessionID(ctx))
	if sessionID == "" {
		return result{}, errors.New("propose_skill: no active session")
	}
	cwd := strings.TrimSpace(executionctx.WorkspaceCWD(ctx, t.defaultCwd))
	if cwd == "" {
		return result{}, errors.New("propose_skill: no active workspace")
	}
	scope, err := parseScope(input.Scope)
	if err != nil {
		return result{}, err
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
		return result{}, fmt.Errorf("propose_skill: invalid proposal: %w", err)
	}
	ref, err := t.proposals.SubmitSkillProposal(ctx, cwd, proposal)
	if err != nil {
		return result{}, fmt.Errorf("propose_skill: submit proposal: %w", err)
	}
	return result{
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
