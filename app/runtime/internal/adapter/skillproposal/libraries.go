// Package skillproposal routes Skill proposal operations to the user or
// project library while keeping filesystem layout outside Application.
package skillproposal

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// Libraries routes proposal operations to the user library or the current
// project's library without exposing filesystem layout to Application.
type Libraries struct {
	user     *skillauthoring.Store
	projects sync.Map
}

func NewLibraries(user *skillauthoring.Store) *Libraries {
	return &Libraries{user: user}
}

func (l *Libraries) SubmitProposal(ctx context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, []string, error) {
	store, err := l.store(proposal.Scope, projectRoot)
	if err != nil {
		return skills.ProposalRef{}, nil, err
	}
	return store.SubmitProposal(ctx, proposal)
}

func (l *Libraries) ListProposals(ctx context.Context, projectRoot string) ([]skills.ProposalReview, error) {
	project, err := l.store(skills.ScopeProject, projectRoot)
	if err != nil {
		return nil, err
	}
	projectProposals, err := project.ListProposals(ctx)
	if err != nil {
		return nil, err
	}
	if l.user == nil || !l.user.Enabled() {
		return projectProposals, nil
	}
	userProposals, err := l.user.ListProposals(ctx)
	if err != nil {
		return nil, err
	}
	return append(projectProposals, userProposals...), nil
}

func (l *Libraries) ApproveProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error) {
	store, err := l.store(ref.Scope, projectRoot)
	if err != nil {
		return nil, err
	}
	return store.ApproveProposal(ctx, ref)
}

func (l *Libraries) RejectProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) ([]string, error) {
	store, err := l.store(ref.Scope, projectRoot)
	if err != nil {
		return nil, err
	}
	return store.RejectProposal(ctx, ref)
}

func (l *Libraries) store(scope skills.Scope, projectRoot string) (*skillauthoring.Store, error) {
	switch scope {
	case skills.ScopeUser:
		if l.user == nil || !l.user.Enabled() {
			return nil, errors.New("skillproposal: user Skill library is unavailable")
		}
		return l.user, nil
	case skills.ScopeProject:
		if strings.TrimSpace(projectRoot) == "" {
			return nil, errors.New("skillproposal: project root is required")
		}
		if loaded, ok := l.projects.Load(projectRoot); ok {
			store, ok := loaded.(*skillauthoring.Store)
			if !ok {
				return nil, errors.New("skillproposal: project Skill cache contains an invalid entry")
			}
			return store, nil
		}
		created := skillauthoring.NewStore(promptsource.ProjectSkillDir(projectRoot), skills.ScopeProject)
		loaded, _ := l.projects.LoadOrStore(projectRoot, created)
		store, ok := loaded.(*skillauthoring.Store)
		if !ok {
			return nil, errors.New("skillproposal: project Skill cache contains an invalid entry")
		}
		return store, nil
	default:
		return nil, errors.New("skillproposal: invalid Skill scope")
	}
}
