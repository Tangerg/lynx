// Package capabilityflow owns human-authored prompt resources and reviewed
// agent capabilities. Filesystem conventions are kept here, outside the
// protocol and execution engine.
package capabilityflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const maxAuthoredDocumentBytes = 1 << 20

type Store interface {
	ListManagedSkillRecords(context.Context) ([]protocol.ManagedSkill, error)
	PutManagedSkill(context.Context, protocol.ManagedSkill) error
	DeleteManagedSkill(context.Context, string) error
	ListSkillProposalRecords(context.Context, string) ([]protocol.SkillProposal, error)
	GetSkillProposalRecord(context.Context, string, string, string) (protocol.SkillProposal, error)
	PutSkillProposalRecord(context.Context, string, protocol.SkillProposal) error
	DeleteSkillProposalRecord(context.Context, string, string, string) error
}

type Resolver interface { Resolve(context.Context, string) (workspacefs.Resolution, error) }
type IDs interface { New(string) (string, error) }

type Service struct {
	store       Store
	resolver    Resolver
	ids         IDs
	home        string
	userRoot    string
	serial      *identityCoordinator
}

func New(store Store, resolver Resolver, ids IDs, home string) (*Service, error) {
	if store == nil || resolver == nil || ids == nil || !filepath.IsAbs(home) {
		return nil, errors.New("capabilityflow: store, resolver, ids and absolute home are required")
	}
	home = filepath.Clean(home)
	if err := prepareUserSkillLibrary(home); err != nil {
		return nil, fmt.Errorf("capabilityflow: prepare user skills: %w", err)
	}
	return &Service{
		store:       store,
		resolver:    resolver,
		ids:         ids,
		home:        home,
		userRoot:    filepath.Join(home, ".lyra"),
		serial:      newIdentityCoordinator(),
	}, nil
}

func (service *Service) resolve(ctx context.Context,workspace *protocol.WorkspaceRef)(workspacefs.Resolution,error){requested:="";if workspace!=nil{requested=workspace.Path};resolved,err:=service.resolver.Resolve(ctx,requested);if err!=nil||!resolved.Available{return workspacefs.Resolution{},protocol.ErrWorkspaceUnavailable};return resolved,nil}
