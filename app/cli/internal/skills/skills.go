// Package skills defines the CLI-owned Skill catalog, curator lifecycle, and
// immutable proposal review port.
package skills

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type Scope string

const (
	ProjectScope Scope = "project"
	UserScope    Scope = "user"
)

func (scope Scope) Validate() error {
	if scope != ProjectScope && scope != UserScope {
		return fmt.Errorf("skill scope %q is invalid", scope)
	}
	return nil
}

type Lifecycle string

const (
	Active   Lifecycle = "active"
	Archived Lifecycle = "archived"
)

func (lifecycle Lifecycle) Validate() error {
	if lifecycle != Active && lifecycle != Archived {
		return fmt.Errorf("skill lifecycle %q is invalid", lifecycle)
	}
	return nil
}

type Origin string

const (
	Requested Origin = "requested"
	Mined     Origin = "mined"
)

func (origin Origin) Validate() error {
	if origin != "" && origin != Requested && origin != Mined {
		return fmt.Errorf("skill proposal origin %q is invalid", origin)
	}
	return nil
}

type Discovered struct {
	Name        string
	Description string
	Scope       Scope
}

func (skill Discovered) Validate() error {
	if strings.TrimSpace(skill.Name) == "" {
		return errors.New("discovered skill name is empty")
	}
	return skill.Scope.Validate()
}

func (skill Discovered) Key() string { return string(skill.Scope) + "/" + skill.Name }

type Managed struct {
	Name        string
	Description string
	Lifecycle   Lifecycle
}

func (skill Managed) Validate() error {
	if strings.TrimSpace(skill.Name) == "" {
		return errors.New("managed skill name is empty")
	}
	return skill.Lifecycle.Validate()
}

type Proposal struct {
	Name          string
	Revision      string
	Scope         Scope
	Description   string
	Instructions  string
	Origin        Origin
	SourceSession string
	Revises       bool
}

func (proposal Proposal) Validate() error {
	if err := validateProposalIdentity(proposal.Name, proposal.Revision, proposal.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(proposal.Description) == "" {
		return errors.New("skill proposal description is empty")
	}
	if strings.TrimSpace(proposal.Instructions) == "" {
		return errors.New("skill proposal instructions are empty")
	}
	return proposal.Origin.Validate()
}

func (proposal Proposal) QualifiedName() string { return string(proposal.Scope) + "/" + proposal.Name }

func (proposal Proposal) Key() string {
	revision := proposal.Revision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return proposal.QualifiedName() + "@" + revision
}

func (proposal Proposal) Reference(workspace string) (ProposalReference, error) {
	reference := ProposalReference{
		Workspace: workspace,
		Name:      proposal.Name,
		Revision:  proposal.Revision,
		Scope:     proposal.Scope,
	}
	return reference, reference.Validate()
}

type ProposalReference struct {
	Workspace string
	Name      string
	Revision  string
	Scope     Scope
}

func (reference ProposalReference) Validate() error {
	if strings.TrimSpace(reference.Workspace) == "" {
		return errors.New("skill proposal reference workspace is empty")
	}
	if err := validateProposalIdentity(reference.Name, reference.Revision, reference.Scope); err != nil {
		return fmt.Errorf("skill proposal reference: %w", err)
	}
	return nil
}

func validateProposalIdentity(name, revision string, scope Scope) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("skill proposal name is empty")
	}
	if len(revision) != 64 {
		return errors.New("skill proposal revision is not a SHA-256 digest")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return fmt.Errorf("skill proposal revision: %w", err)
	}
	return scope.Validate()
}

type Service interface {
	Discover(context.Context, string) ([]Discovered, error)
	Managed(context.Context) ([]Managed, error)
	Proposals(context.Context, string) ([]Proposal, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) error
	Approve(context.Context, ProposalReference) error
	Reject(context.Context, ProposalReference) error
}
