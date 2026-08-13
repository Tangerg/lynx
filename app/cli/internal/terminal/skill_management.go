package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/skills"
)

func (a *app) ShowDiscoveredSkills() {
	if a.skills == nil {
		a.message("this runtime composition has no skill service")
		return
	}
	a.executeRuntimeReaderQuery(a.discoveredSkillsReaderQuery())
}

func (a *app) discoveredSkillsReaderQuery() runtimeReaderQuery {
	workspace := a.session.Workspace.Path
	return runtimeReaderQuery{
		status: "loading discovered skills",
		mode:   runtimeReaderDiscoveredSkills,
		read: func(ctx context.Context) (readerDocument, error) {
			discovered, err := a.skills.Discover(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return discoveredSkillsDocument(workspace, discovered), nil
		},
	}
}

func discoveredSkillsDocument(workspace string, discovered []skills.Discovered) readerDocument {
	lines := make([]string, 0, len(discovered))
	for _, skill := range discovered {
		line := skill.Key()
		if skill.Description != "" {
			line += "  " + skill.Description
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "No skills are discoverable for this workspace.")
	}
	return paragraphDocument("Discovered skills", fmt.Sprintf("%d available · %s", len(discovered), workspace), lines)
}

func (a *app) ShowManagedSkills() {
	if a.skills == nil {
		a.message("this runtime composition has no skill service")
		return
	}
	a.executeRuntimeReaderQuery(a.managedSkillsReaderQuery())
}

func (a *app) managedSkillsReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading managed skills",
		mode:   runtimeReaderManagedSkills,
		read: func(ctx context.Context) (readerDocument, error) {
			managed, err := a.skills.Managed(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return managedSkillsDocument(managed), nil
		},
	}
}

func managedSkillsDocument(managed []skills.Managed) readerDocument {
	lines := make([]string, 0, len(managed))
	for _, skill := range managed {
		line := skill.Name + "  " + string(skill.Lifecycle)
		if skill.Description != "" {
			line += "  · " + skill.Description
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "The managed user Skill library is empty.")
	}
	return paragraphDocument("Managed Skill library", fmt.Sprintf("%d entries", len(managed)), lines)
}

func (a *app) ShowSkillProposals() {
	if a.skills == nil {
		a.message("this runtime composition has no skill service")
		return
	}
	a.executeRuntimeReaderQuery(a.skillProposalsReaderQuery())
}

func (a *app) skillProposalsReaderQuery() runtimeReaderQuery {
	workspace := a.session.Workspace.Path
	return runtimeReaderQuery{
		status: "loading skill proposals",
		mode:   runtimeReaderSkillProposals,
		read: func(ctx context.Context) (readerDocument, error) {
			proposals, err := a.skills.Proposals(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return skillProposalsDocument(proposals), nil
		},
	}
}

func skillProposalsDocument(proposals []skills.Proposal) readerDocument {
	if len(proposals) == 0 {
		return paragraphDocument("Skill proposals", "none pending", []string{"No Skill proposals are awaiting review."})
	}
	sections := make([]ToolSection, 0, len(proposals)*2)
	for _, proposal := range proposals {
		provenance := string(proposal.Origin)
		if provenance == "" {
			provenance = "unspecified origin"
		}
		if proposal.Revises {
			provenance += " · revises existing skill"
		}
		if proposal.SourceSession != "" {
			provenance += " · session " + shortIdentity(proposal.SourceSession)
		}
		sections = append(sections,
			ToolSection{Title: proposal.Key(), Style: toolSectionParagraph, Text: proposal.Description + "\n" + provenance},
			ToolSection{Title: "Instructions", Style: toolSectionParagraph, Text: proposal.Instructions, Links: true},
		)
	}
	return readerDocument{Title: "Skill proposals", Detail: fmt.Sprintf("%d awaiting review", len(proposals)), Sections: sections}
}

func (a *app) ArchiveSkill(name string) error {
	if a.skills == nil {
		return errors.New("this runtime composition has no skill service")
	}
	return a.changeSkillLifecycle("archiving skill", name, a.skills.Archive)
}

func (a *app) RestoreSkill(name string) error {
	if a.skills == nil {
		return errors.New("this runtime composition has no skill service")
	}
	return a.changeSkillLifecycle("restoring skill", name, a.skills.Restore)
}

func (a *app) changeSkillLifecycle(
	status, name string,
	change func(context.Context, string) error,
) error {
	if a.skills == nil {
		return errors.New("this runtime composition has no skill service")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a skill name is required")
	}
	a.status.note(status + " " + name)
	started := runApplicationOperation(a, skillOperation, false,
		func(ctx context.Context) (string, error) { return name, change(ctx, name) },
		func(changed string, err error) {
			if err != nil {
				a.message(status + " failed: " + err.Error())
				return
			}
			a.message(status + " complete · " + changed)
		},
	)
	if !started {
		return errors.New("another skill operation is running")
	}
	return nil
}

type skillProposalDecision struct {
	proposal  skills.Proposal
	reference skills.ProposalReference
}

func (a *app) PrepareSkillProposalDecision(identity string, approve bool) error {
	if a.skills == nil {
		return errors.New("this runtime composition has no skill service")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("a proposal name or scope/name is required")
	}
	workspace := a.session.Workspace.Path
	a.status.note("loading skill proposal " + identity)
	started := runOperation(a, skillOperation, false,
		func(ctx context.Context) (skillProposalDecision, error) {
			proposals, err := a.skills.Proposals(ctx, workspace)
			if err != nil {
				return skillProposalDecision{}, err
			}
			proposal, err := resolveSkillProposal(proposals, identity)
			if err != nil {
				return skillProposalDecision{}, err
			}
			reference, err := proposal.Reference(workspace)
			return skillProposalDecision{proposal: proposal, reference: reference}, err
		},
		func(decision skillProposalDecision, err error) {
			if err != nil {
				a.message("load skill proposal failed: " + err.Error())
				return
			}
			a.confirmSkillProposalDecision(decision, approve)
		},
	)
	if !started {
		return errors.New("another skill operation is running")
	}
	return nil
}

func resolveSkillProposal(proposals []skills.Proposal, identity string) (skills.Proposal, error) {
	matches := make([]skills.Proposal, 0, 1)
	for _, proposal := range proposals {
		if proposal.Key() == identity || proposal.QualifiedName() == identity || proposal.Name == identity {
			matches = append(matches, proposal)
		}
	}
	switch len(matches) {
	case 0:
		return skills.Proposal{}, errors.New("skill proposal not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return skills.Proposal{}, errors.New("skill proposal name is ambiguous; use project/name or user/name")
	}
}

func (a *app) confirmSkillProposalDecision(decision skillProposalDecision, approve bool) {
	verb, action := "Reject", "Reject permanently"
	if approve {
		verb, action = "Approve", "Approve and publish"
	}
	question := fmt.Sprintf("%s %s? %s", verb, decision.proposal.Key(), decision.proposal.Description)
	a.confirmAction(verb+" Skill proposal", question, action, func() {
		a.decideSkillProposal(decision.reference, approve)
	})
}

func (a *app) decideSkillProposal(reference skills.ProposalReference, approve bool) {
	verb := "rejecting"
	decide := a.skills.Reject
	if approve {
		verb = "approving"
		decide = a.skills.Approve
	}
	a.status.note(verb + " skill proposal " + reference.Name)
	started := runApplicationOperation(a, skillOperation, false,
		func(ctx context.Context) (skills.ProposalReference, error) { return reference, decide(ctx, reference) },
		func(reviewed skills.ProposalReference, err error) {
			if err != nil {
				a.message(verb + " skill proposal failed: " + err.Error())
				return
			}
			a.message(verb + " skill proposal complete · " + string(reviewed.Scope) + "/" + reviewed.Name)
		},
	)
	if !started {
		a.message("another skill operation is running")
	}
}
