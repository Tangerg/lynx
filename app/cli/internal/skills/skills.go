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

func (s Scope) Validate() error {
	if s != ProjectScope && s != UserScope {
		return fmt.Errorf("skill scope %q is invalid", s)
	}
	return nil
}

type Lifecycle string

const (
	Active   Lifecycle = "active"
	Archived Lifecycle = "archived"
)

func (l Lifecycle) Validate() error {
	if l != Active && l != Archived {
		return fmt.Errorf("skill lifecycle %q is invalid", l)
	}
	return nil
}

type Origin string

const (
	Requested Origin = "requested"
	Mined     Origin = "mined"
)

func (o Origin) Validate() error {
	if o != "" && o != Requested && o != Mined {
		return fmt.Errorf("skill proposal origin %q is invalid", o)
	}
	return nil
}

type Discovered struct {
	Name        string
	Description string
	Scope       Scope
}

func (d Discovered) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("discovered skill name is empty")
	}
	return d.Scope.Validate()
}

func (d Discovered) Key() string { return string(d.Scope) + "/" + d.Name }

type Managed struct {
	Name        string
	Description string
	Lifecycle   Lifecycle
}

func (m Managed) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("managed skill name is empty")
	}
	return m.Lifecycle.Validate()
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

func (p Proposal) Validate() error {
	if err := validateProposalIdentity(p.Name, p.Revision, p.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(p.Description) == "" {
		return errors.New("skill proposal description is empty")
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("skill proposal instructions are empty")
	}
	return p.Origin.Validate()
}

func (p Proposal) QualifiedName() string { return string(p.Scope) + "/" + p.Name }

func (p Proposal) Key() string {
	revision := p.Revision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return p.QualifiedName() + "@" + revision
}

func (p Proposal) Reference(workspace string) (ProposalReference, error) {
	reference := ProposalReference{
		Workspace: workspace,
		Name:      p.Name,
		Revision:  p.Revision,
		Scope:     p.Scope,
	}
	return reference, reference.Validate()
}

type ProposalReference struct {
	Workspace string
	Name      string
	Revision  string
	Scope     Scope
}

func (p ProposalReference) Validate() error {
	if strings.TrimSpace(p.Workspace) == "" {
		return errors.New("skill proposal reference workspace is empty")
	}
	if err := validateProposalIdentity(p.Name, p.Revision, p.Scope); err != nil {
		return fmt.Errorf("skill proposal reference: %w", err)
	}
	return nil
}

// ValidateDecisionAcknowledgement proves that the exact immutable proposal
// reviewed by Approve or Reject is no longer pending. Other revisions of the
// same skill remain independent proposals.
func (p ProposalReference) ValidateDecisionAcknowledgement(pending []Proposal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	for index, proposal := range pending {
		if err := proposal.Validate(); err != nil {
			return fmt.Errorf("skill proposal acknowledgement item %d: %w", index+1, err)
		}
		if proposal.Name == p.Name && proposal.Scope == p.Scope && proposal.Revision == p.Revision {
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
