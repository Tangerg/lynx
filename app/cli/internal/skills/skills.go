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

// ValidateLifecycleAcknowledgement proves that an authoritative managed-skill
// catalog reflects the requested lifecycle for exactly one named skill.
func ValidateLifecycleAcknowledgement(catalog []Managed, name string, lifecycle Lifecycle) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed skill acknowledgement name is empty")
	}
	if err := lifecycle.Validate(); err != nil {
		return err
	}
	found := false
	for index, skill := range catalog {
		if err := skill.Validate(); err != nil {
			return fmt.Errorf("managed skill acknowledgement item %d: %w", index+1, err)
		}
		if skill.Name != name {
			continue
		}
		if found {
			return fmt.Errorf("managed skill acknowledgement repeats %q", name)
		}
		found = true
		if skill.Lifecycle != lifecycle {
			return fmt.Errorf("managed skill %q lifecycle is %q, want %q", name, skill.Lifecycle, lifecycle)
		}
	}
	if !found {
		return fmt.Errorf("managed skill %q is missing after lifecycle change", name)
	}
	return nil
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

// ValidateDecisionAcknowledgement proves that the exact immutable proposal
// reviewed by Approve or Reject is no longer pending. Other revisions of the
// same skill remain independent proposals.
func (reference ProposalReference) ValidateDecisionAcknowledgement(pending []Proposal) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	for index, proposal := range pending {
		if err := proposal.Validate(); err != nil {
			return fmt.Errorf("skill proposal acknowledgement item %d: %w", index+1, err)
		}
		if proposal.Name == reference.Name && proposal.Scope == reference.Scope && proposal.Revision == reference.Revision {
			return fmt.Errorf("skill proposal %s remains pending after decision", proposal.Key())
		}
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
