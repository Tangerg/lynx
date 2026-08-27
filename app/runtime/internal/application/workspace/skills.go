package workspace

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/skills"
)

var (
	// ErrSkillProposalsUnavailable reports that proposal review is unavailable.
	ErrSkillProposalsUnavailable = errors.New("workspace: skill proposals unavailable")
	// ErrSkillLibraryUnavailable reports that Skill-library curation is unavailable.
	ErrSkillLibraryUnavailable = errors.New("workspace: skill library unavailable")
)

// SkillCatalog enumerates skills visible from a working directory.
type SkillCatalog interface {
	List(ctx context.Context, cwd string) ([]SkillSummary, error)
}

// SkillCurator manages active and archived user-authored Skills. Mutation
// methods return the exact opaque file identities whose public projection
// changed, including changes committed before a later error.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) ([]string, error)
	Restore(ctx context.Context, name string) ([]string, error)
}

// SkillProposals stores immutable project or user proposals. Mutation methods
// return exact opaque file identities so filesystem observation can accept only
// the caller's committed paths without swallowing concurrent external edits.
type SkillProposals interface {
	SubmitProposal(ctx context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, []string, error)
	ListProposals(ctx context.Context, projectRoot string) ([]skills.ProposalReview, error)
	ApproveProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error)
	RejectProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error)
}

// Skills owns discovery, library curation, and proposal review.
type Skills struct {
	scope         *Scope
	catalog       SkillCatalog
	curator       SkillCurator
	proposals     SkillProposals
	observations  *AuthoredWatch
	invalidations invalidation.Publish
}

// NewSkills builds interactive Skill discovery, curation, and review use cases.
func NewSkills(scope *Scope, catalog SkillCatalog, curator SkillCurator, proposals SkillProposals, observations *AuthoredWatch, invalidations invalidation.Publish) *Skills {
	return &Skills{
		scope: scope, catalog: catalog, curator: curator, proposals: proposals,
		observations: observations, invalidations: invalidations,
	}
}

// List enumerates the Skills visible from cwd.
func (s *Skills) List(ctx context.Context, cwd string) ([]SkillSummary, error) {
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
	identities, err := s.curator.Archive(ctx, name)
	s.publishSkillMutation(identities)
	return err
}

// Restore returns an archived Skill to active use.
func (s *Skills) Restore(ctx context.Context, name string) error {
	if s.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	identities, err := s.curator.Restore(ctx, name)
	s.publishSkillMutation(identities)
	return err
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
	ref, identities, err := s.proposals.SubmitProposal(ctx, root, proposal)
	s.publishSkillMutation(identities)
	return ref, err
}

// Proposals returns immutable Skill proposals visible from cwd.
func (s *Skills) Proposals(ctx context.Context, cwd string) ([]skills.ProposalReview, error) {
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
	identities, err := s.proposals.ApproveProposal(ctx, root, ref)
	s.publishSkillMutation(identities)
	return err
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
	identities, err := s.proposals.RejectProposal(ctx, root, ref)
	s.publishSkillMutation(identities)
	return err
}

func (s *Skills) publishSkillMutation(identities []string) {
	if len(identities) == 0 {
		return
	}
	if s.observations != nil {
		s.observations.Accept(AuthoredChange{Resource: AuthoredSkills, Identities: identities})
	}
	s.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
}
