package workspace

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

var (
	// ErrSkillProposalsUnavailable reports that proposal review is unavailable.
	ErrSkillProposalsUnavailable = errors.New("workspace: skill proposals unavailable")
	// ErrSkillLibraryUnavailable reports that Skill-library curation is unavailable.
	ErrSkillLibraryUnavailable = errors.New("workspace: skill library unavailable")
)

// SkillCatalog enumerates skills visible from a working directory.
type SkillCatalog interface {
	List(ctx context.Context, cwd string) ([]SkillInfo, error)
}

// SkillCurator manages active and archived user-authored Skills.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) error
	Restore(ctx context.Context, name string) error
}

// SkillProposals stores immutable project or user proposals.
type SkillProposals interface {
	SubmitProposal(ctx context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, error)
	ListProposals(ctx context.Context, projectRoot string) ([]skills.ProposalInfo, error)
	ApproveProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) error
	RejectProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) error
}

// Skills owns discovery, library curation, and proposal review.
type Skills struct {
	scope         *Scope
	catalog       SkillCatalog
	curator       SkillCurator
	proposals     SkillProposals
	skillsChanged func(struct{})
}

func NewSkills(scope *Scope, catalog SkillCatalog, curator SkillCurator, proposals SkillProposals, changed func(struct{})) *Skills {
	return &Skills{scope: scope, catalog: catalog, curator: curator, proposals: proposals, skillsChanged: changed}
}

// List enumerates the Skills visible from cwd.
func (s *Skills) List(ctx context.Context, cwd string) ([]SkillInfo, error) {
	root, err := s.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if s.catalog == nil {
		return nil, nil
	}
	return s.catalog.List(ctx, root)
}

// Managed returns active and archived user-authored Skills.
func (s *Skills) Managed(ctx context.Context) ([]skills.Entry, error) {
	if s.curator == nil {
		return nil, ErrSkillLibraryUnavailable
	}
	return s.curator.List(ctx)
}

// Archive removes a Skill from active use without deleting it.
func (s *Skills) Archive(ctx context.Context, name string) error {
	if s.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	if err := s.curator.Archive(ctx, name); err != nil {
		return err
	}
	s.notifySkillsChanged()
	return nil
}

// Restore returns an archived Skill to active use.
func (s *Skills) Restore(ctx context.Context, name string) error {
	if s.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	if err := s.curator.Restore(ctx, name); err != nil {
		return err
	}
	s.notifySkillsChanged()
	return nil
}

// SubmitProposal submits immutable Skill content without activating it.
func (s *Skills) SubmitProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	if s.proposals == nil {
		return skills.ProposalRef{}, ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	ref, err := s.proposals.SubmitProposal(ctx, root, proposal)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	s.notifySkillsChanged()
	return ref, nil
}

// Proposals returns immutable Skill proposals visible from cwd.
func (s *Skills) Proposals(ctx context.Context, cwd string) ([]skills.ProposalInfo, error) {
	if s.proposals == nil {
		return nil, ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	return s.proposals.ListProposals(ctx, root)
}

// ApproveProposal accepts a Skill proposal into its target library.
func (s *Skills) ApproveProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if s.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return err
	}
	if err := s.proposals.ApproveProposal(ctx, root, ref); err != nil {
		return err
	}
	s.notifySkillsChanged()
	return nil
}

// RejectProposal removes a Skill proposal without activating it.
func (s *Skills) RejectProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if s.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := s.scope.root(cwd)
	if err != nil {
		return err
	}
	if err := s.proposals.RejectProposal(ctx, root, ref); err != nil {
		return err
	}
	s.notifySkillsChanged()
	return nil
}

func (s *Skills) notifySkillsChanged() {
	if s.skillsChanged != nil {
		s.skillsChanged(struct{}{})
	}
}
