package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/skills"
)

type skillBinding interface {
	ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.Skill], error)
	ListManagedSkills(context.Context, embedded.CallOptions) (*protocol.Page[protocol.ManagedSkill], error)
	ArchiveSkill(context.Context, protocol.SkillNameRequest, embedded.CommandOptions) error
	RestoreSkill(context.Context, protocol.SkillNameRequest, embedded.CommandOptions) error
	ListSkillProposals(context.Context, protocol.WorkspaceQuery, embedded.CallOptions) (*protocol.Page[protocol.SkillProposal], error)
	ApproveSkillProposal(context.Context, protocol.SkillProposalRef, embedded.CommandOptions) error
	RejectSkillProposal(context.Context, protocol.SkillProposalRef, embedded.CommandOptions) error
}

var _ skills.Service = (*Runtime)(nil)

func (r *Runtime) Discover(ctx context.Context, workspace string) ([]skills.Discovered, error) {
	query, err := skillWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.skills.ListDiscoveredSkills(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list discovered skills: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list discovered skills: runtime returned an unsupported continuation cursor")
	}
	projected := make([]skills.Discovered, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for index, value := range page.Data {
		skill := skills.Discovered{
			Name: value.Name, Description: value.Description, Scope: skills.Scope(value.Scope),
		}
		if err := skill.Validate(); err != nil {
			return nil, fmt.Errorf("list discovered skills item %d: %w", index+1, err)
		}
		if _, duplicate := seen[skill.Key()]; duplicate {
			return nil, fmt.Errorf("list discovered skills repeats %q", skill.Key())
		}
		seen[skill.Key()] = struct{}{}
		projected = append(projected, skill)
	}
	return projected, nil
}

func (r *Runtime) Managed(ctx context.Context) ([]skills.Managed, error) {
	page, err := r.skills.ListManagedSkills(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list managed skills: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list managed skills: runtime returned an unsupported continuation cursor")
	}
	projected := make([]skills.Managed, 0, len(page.Data))
	seen := make(map[string]struct{}, len(page.Data))
	for index, value := range page.Data {
		skill := skills.Managed{
			Name: value.Name, Description: value.Description, Lifecycle: skills.Lifecycle(value.Lifecycle),
		}
		if err := skill.Validate(); err != nil {
			return nil, fmt.Errorf("list managed skills item %d: %w", index+1, err)
		}
		if _, duplicate := seen[skill.Name]; duplicate {
			return nil, fmt.Errorf("list managed skills repeats %q", skill.Name)
		}
		seen[skill.Name] = struct{}{}
		projected = append(projected, skill)
	}
	return projected, nil
}

func (r *Runtime) Proposals(ctx context.Context, workspace string) ([]skills.Proposal, error) {
	query, err := skillWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.skills.ListSkillProposals(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if page == nil {
		return nil, errors.New("list skill proposals: runtime returned nil")
	}
	if page.NextCursor != "" {
		return nil, errors.New("list skill proposals: runtime returned an unsupported continuation cursor")
	}
	projected := make([]skills.Proposal, 0, len(page.Data))
	seen := make(map[[3]string]struct{}, len(page.Data))
	for index, value := range page.Data {
		proposal := skills.Proposal{
			Name: value.Name, Revision: value.Revision, Scope: skills.Scope(value.Scope),
			Description: value.Description, Instructions: value.Instructions,
			Origin: skills.Origin(value.Origin), SourceSession: value.SourceSession, Revises: value.Revises,
		}
		if err := proposal.Validate(); err != nil {
			return nil, fmt.Errorf("list skill proposals item %d: %w", index+1, err)
		}
		identity := [3]string{string(proposal.Scope), proposal.Name, proposal.Revision}
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("list skill proposals repeats %q", proposal.Key())
		}
		seen[identity] = struct{}{}
		projected = append(projected, proposal)
	}
	return projected, nil
}

func (r *Runtime) Archive(ctx context.Context, name string) error {
	return r.changeSkillLifecycle(ctx, "archive skill", name, r.skills.ArchiveSkill)
}

func (r *Runtime) Restore(ctx context.Context, name string) error {
	return r.changeSkillLifecycle(ctx, "restore skill", name, r.skills.RestoreSkill)
}

func (r *Runtime) changeSkillLifecycle(
	ctx context.Context,
	operation, name string,
	change func(context.Context, protocol.SkillNameRequest, embedded.CommandOptions) error,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s: skill name is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(change(ctx, protocol.SkillNameRequest{Name: name}, options))
}

func (r *Runtime) Approve(ctx context.Context, reference skills.ProposalReference) error {
	return r.decideSkillProposal(ctx, "approve skill proposal", reference, r.skills.ApproveSkillProposal)
}

func (r *Runtime) Reject(ctx context.Context, reference skills.ProposalReference) error {
	return r.decideSkillProposal(ctx, "reject skill proposal", reference, r.skills.RejectSkillProposal)
}

func (r *Runtime) decideSkillProposal(
	ctx context.Context,
	operation string,
	reference skills.ProposalReference,
	decide func(context.Context, protocol.SkillProposalRef, embedded.CommandOptions) error,
) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	request := protocol.SkillProposalRef{
		Workspace: protocol.WorkspaceRef{Path: reference.Workspace},
		Name:      reference.Name, Revision: reference.Revision, Scope: protocol.SkillScope(reference.Scope),
	}
	if err := decide(ctx, request, options); err != nil {
		return classifyError(err)
	}
	return nil
}

func skillWorkspaceQuery(workspace string) (protocol.WorkspaceQuery, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return protocol.WorkspaceQuery{}, errors.New("skills: workspace is empty")
	}
	return protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, nil
}
