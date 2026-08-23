package capabilityflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	lyraskills "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

// SkillProposalDraft is trusted Runtime provenance plus model-authored Skill
// content. It is deliberately not a wire type: the Agent can only submit a
// pending review, while publication remains a human operation.
type SkillProposalDraft struct {
	Workspace     protocol.WorkspaceRef
	SourceSession string
	Name          string
	Description   string
	Instructions  string
	Scope         protocol.SkillScope
	Origin        protocol.SkillProposalOrigin
}

func (service *Service) DiscoveredSkills(
	ctx context.Context,
	query protocol.WorkspaceQuery,
) (*protocol.Page[protocol.Skill], error) {
	resolved, err := service.resolve(ctx, &query.Workspace)
	if err != nil {
		return nil, err
	}
	project, user, err := service.skillSources(ctx, resolved)
	if err != nil {
		return nil, err
	}
	projectSkills, err := project.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: list project skills: %w", err)
	}
	userSkills, err := user.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: list user skills: %w", err)
	}

	values := make([]protocol.Skill, 0, len(projectSkills)+len(userSkills))
	seen := make(map[string]struct{}, len(projectSkills)+len(userSkills))
	for _, summary := range projectSkills {
		seen[summary.Name] = struct{}{}
		values = append(values, protocol.Skill{
			Name: summary.Name, Description: summary.Description, Scope: protocol.SkillScopeProject,
		})
	}
	for _, summary := range userSkills {
		if _, shadowed := seen[summary.Name]; shadowed {
			continue
		}
		values = append(values, protocol.Skill{
			Name: summary.Name, Description: summary.Description, Scope: protocol.SkillScopeUser,
		})
	}
	slices.SortFunc(values, func(left, right protocol.Skill) int {
		return strings.Compare(left.Name, right.Name)
	})
	return protocol.NewPage(values), nil
}

// SkillSource returns the exact progressive-disclosure source used by an
// Agent Run. Project Skills win by name; archived user Skills are invisible.
func (service *Service) SkillSource(
	ctx context.Context,
	workspace string,
) (lyraskills.ResourceSource, error) {
	resolved, err := service.resolve(ctx, &protocol.WorkspaceRef{Path: workspace})
	if err != nil {
		return nil, err
	}
	project, user, err := service.skillSources(ctx, resolved)
	if err != nil {
		return nil, err
	}
	return lyraskills.Merge(project, user), nil
}

func (service *Service) skillSources(
	ctx context.Context,
	resolved workspacefs.Resolution,
) (lyraskills.ResourceSource, lyraskills.ResourceSource, error) {
	projectRoot, err := confinedSkillLibrary(resolved.Workspace.Path())
	if err != nil {
		return nil, nil, fmt.Errorf("capabilityflow: project skill library: %w", err)
	}
	userRoot, err := confinedSkillLibrary(service.home)
	if err != nil {
		return nil, nil, fmt.Errorf("capabilityflow: user skill library: %w", err)
	}
	release, err := service.serial.Acquire(ctx, userSkillLibraryLane)
	if err != nil {
		return nil, nil, err
	}
	records, err := service.reconcileManagedSkills(ctx, userRoot)
	release()
	if err != nil {
		return nil, nil, err
	}
	archived := make(map[string]struct{})
	for _, record := range records {
		if record.Lifecycle == protocol.SkillLifecycleArchived {
			archived[record.Name] = struct{}{}
		}
	}
	return lyraskills.Dir(projectRoot), &curatedSkillSource{
		source: lyraskills.Dir(userRoot), archived: archived,
	}, nil
}

func (service *Service) ManagedSkills(ctx context.Context) (*protocol.Page[protocol.ManagedSkill], error) {
	release, err := service.serial.Acquire(ctx, userSkillLibraryLane)
	if err != nil {
		return nil, err
	}
	defer release()

	root, err := confinedSkillLibrary(service.home)
	if err != nil {
		return nil, err
	}
	values, err := service.reconcileManagedSkills(ctx, root)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(values), nil
}

func (service *Service) reconcileManagedSkills(
	ctx context.Context,
	root string,
) ([]protocol.ManagedSkill, error) {
	summaries, err := lyraskills.Dir(root).List(ctx)
	if err != nil {
		return nil, fmt.Errorf("capabilityflow: list managed skills: %w", err)
	}
	records, err := service.store.ListManagedSkillRecords(ctx)
	if err != nil {
		return nil, err
	}
	recordByName := make(map[string]protocol.ManagedSkill, len(records))
	for _, record := range records {
		recordByName[record.Name] = record
	}

	values := make([]protocol.ManagedSkill, 0, len(summaries))
	for _, summary := range summaries {
		record, known := recordByName[summary.Name]
		lifecycle := record.Lifecycle
		if lifecycle != protocol.SkillLifecycleArchived {
			lifecycle = protocol.SkillLifecycleActive
		}
		value := protocol.ManagedSkill{
			Name: summary.Name, Description: summary.Description, Lifecycle: lifecycle,
		}
		values = append(values, value)
		delete(recordByName, summary.Name)
		if !known || record != value {
			if err := service.store.PutManagedSkill(ctx, value); err != nil {
				return nil, err
			}
		}
	}
	for stale := range recordByName {
		if err := service.store.DeleteManagedSkill(ctx, stale); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (service *Service) SetSkillLifecycle(
	ctx context.Context,
	request protocol.SkillNameRequest,
	lifecycle protocol.SkillLifecycle,
) error {
	if lifecycle != protocol.SkillLifecycleActive && lifecycle != protocol.SkillLifecycleArchived {
		return fmt.Errorf("%w: invalid skill lifecycle", protocol.ErrInvalidParams)
	}
	if err := (lyraskills.Frontmatter{
		Name: request.Name, Description: "managed Skill reference",
	}).Validate(); err != nil {
		return fmt.Errorf("%w: invalid Skill name: %v", protocol.ErrInvalidParams, err)
	}
	release, err := service.serial.Acquire(ctx, userSkillLibraryLane)
	if err != nil {
		return err
	}
	defer release()

	root, err := confinedSkillLibrary(service.home)
	if err != nil {
		return err
	}
	skill, err := lyraskills.Dir(root).Load(ctx, request.Name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, lyraskills.ErrInvalidSkill) {
			return protocol.ErrItemNotFound
		}
		return err
	}
	return service.store.PutManagedSkill(ctx, protocol.ManagedSkill{
		Name: skill.Name, Description: skill.Description, Lifecycle: lifecycle,
	})
}

func (service *Service) ProposeSkill(
	ctx context.Context,
	draft SkillProposalDraft,
) (*protocol.SkillProposal, error) {
	resolved, err := service.resolve(ctx, &draft.Workspace)
	if err != nil {
		return nil, err
	}
	proposal := protocol.SkillProposal{
		Name:          strings.TrimSpace(draft.Name),
		Description:   strings.TrimSpace(draft.Description),
		Instructions:  strings.TrimSpace(draft.Instructions),
		Scope:         draft.Scope,
		Origin:        draft.Origin,
		SourceSession: strings.TrimSpace(draft.SourceSession),
	}
	document, err := validateSkillProposal(proposal)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	lanes := []string{skillProposalLane(resolved.Workspace.Path(), proposal.Name)}
	if proposal.Scope == protocol.SkillScopeUser {
		lanes = append(lanes, userSkillLibraryLane)
	}
	release, err := service.serial.Acquire(ctx, lanes...)
	if err != nil {
		return nil, err
	}
	defer release()
	library, err := service.skillLibrary(resolved, proposal.Scope)
	if err != nil {
		return nil, err
	}
	_, exists, err := loadSkill(ctx, library.path, proposal.Name)
	if err != nil {
		return nil, err
	}
	proposal.Revises = exists
	proposal.Revision = skillProposalRevision(proposal.Scope, proposal.Name, document)
	if err := service.store.PutSkillProposalRecord(ctx, resolved.Workspace.Path(), proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
}

func (service *Service) SkillProposals(
	ctx context.Context,
	query protocol.WorkspaceQuery,
) (*protocol.Page[protocol.SkillProposal], error) {
	resolved, err := service.resolve(ctx, &query.Workspace)
	if err != nil {
		return nil, err
	}
	values, err := service.store.ListSkillProposalRecords(ctx, resolved.Workspace.Path())
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(values), nil
}

func (service *Service) ApproveProposal(ctx context.Context, ref protocol.SkillProposalRef) error {
	resolved, err := service.resolve(ctx, &ref.Workspace)
	if err != nil {
		return err
	}
	if err := validateSkillProposalRef(ref); err != nil {
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}

	lanes := []string{skillProposalLane(resolved.Workspace.Path(), ref.Name)}
	if ref.Scope == protocol.SkillScopeUser {
		lanes = append(lanes, userSkillLibraryLane)
	}
	release, err := service.serial.Acquire(ctx, lanes...)
	if err != nil {
		return err
	}
	defer release()
	proposal, err := service.store.GetSkillProposalRecord(
		ctx, resolved.Workspace.Path(), ref.Name, ref.Revision,
	)
	if err != nil {
		if errors.Is(err, protocol.ErrItemNotFound) {
			return protocol.ErrItemNotFound
		}
		return err
	}
	if proposal.Scope != ref.Scope || proposal.Name != ref.Name || proposal.Revision != ref.Revision {
		return protocol.ErrRevisionConflict
	}
	document, err := validateSkillProposal(proposal)
	if err != nil {
		return fmt.Errorf("capabilityflow: stored skill proposal is invalid: %w", err)
	}
	if skillProposalRevision(proposal.Scope, proposal.Name, document) != proposal.Revision {
		return protocol.ErrRevisionConflict
	}
	library, err := service.skillLibrary(resolved, proposal.Scope)
	if err != nil {
		return err
	}
	current, exists, err := loadSkill(ctx, library.path, proposal.Name)
	if err != nil {
		return err
	}
	alreadyPublished := exists && skillMatchesProposal(current, proposal)
	if !alreadyPublished {
		if exists != proposal.Revises {
			return protocol.ErrRevisionConflict
		}
		if err := publishSkill(library, proposal.Name, document); err != nil {
			return fmt.Errorf("capabilityflow: publish skill: %w", err)
		}
	}
	if proposal.Scope == protocol.SkillScopeUser {
		if err := service.store.PutManagedSkill(ctx, protocol.ManagedSkill{
			Name: proposal.Name, Description: proposal.Description, Lifecycle: protocol.SkillLifecycleActive,
		}); err != nil {
			return err
		}
	}
	if err := service.store.DeleteSkillProposalRecord(
		ctx, resolved.Workspace.Path(), proposal.Name, proposal.Revision,
	); err != nil {
		if errors.Is(err, protocol.ErrItemNotFound) {
			return protocol.ErrItemNotFound
		}
		return err
	}
	return nil
}

func (service *Service) RejectProposal(ctx context.Context, ref protocol.SkillProposalRef) error {
	resolved, err := service.resolve(ctx, &ref.Workspace)
	if err != nil {
		return err
	}
	if err := validateSkillProposalRef(ref); err != nil {
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	release, err := service.serial.Acquire(
		ctx, skillProposalLane(resolved.Workspace.Path(), ref.Name),
	)
	if err != nil {
		return err
	}
	defer release()
	proposal, err := service.store.GetSkillProposalRecord(
		ctx, resolved.Workspace.Path(), ref.Name, ref.Revision,
	)
	if err != nil {
		if errors.Is(err, protocol.ErrItemNotFound) {
			return protocol.ErrItemNotFound
		}
		return err
	}
	if proposal.Scope != ref.Scope || proposal.Name != ref.Name || proposal.Revision != ref.Revision {
		return protocol.ErrRevisionConflict
	}
	if err := service.store.DeleteSkillProposalRecord(
		ctx, resolved.Workspace.Path(), ref.Name, ref.Revision,
	); err != nil {
		if errors.Is(err, protocol.ErrItemNotFound) {
			return protocol.ErrItemNotFound
		}
		return err
	}
	return nil
}
