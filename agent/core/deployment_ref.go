package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// DeploymentRef is the durable identity of one compiled agent definition. Name is
// the human routing key, Version is the caller-supplied semantic version (and
// may be empty for non-durable in-memory use), and Digest identifies the exact
// canonical declaration plus the host BuildID used at deployment.
//
// DeploymentRef is a comparable value and is safe to use as a map key. It contains
// no runtime pointers and is the identity carried by processes, snapshots, and
// framework events.
type DeploymentRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Digest  string `json:"digest"`
}

// Validate reports whether ref can identify a compiled deployment. Name and
// Digest must not contain surrounding whitespace; a non-empty Version must be
// canonical SemVer in MAJOR.MINOR.PATCH form.
func (r DeploymentRef) Validate() error {
	if r.Name == "" {
		return errors.New("deployment ref: name is empty")
	}
	if strings.TrimSpace(r.Name) != r.Name {
		return fmt.Errorf("deployment ref: name %q has surrounding whitespace", r.Name)
	}
	if r.Version != "" {
		if _, err := semver.StrictNewVersion(r.Version); err != nil {
			return fmt.Errorf("deployment ref %q: invalid version %q: %w", r.Name, r.Version, err)
		}
	}
	if r.Digest == "" {
		return fmt.Errorf("deployment ref %q: digest is empty", r.Name)
	}
	if strings.TrimSpace(r.Digest) != r.Digest {
		return fmt.Errorf("deployment ref %q: digest has surrounding whitespace", r.Name)
	}
	return nil
}

// String returns a compact diagnostic representation. It is not a wire
// encoding; persist DeploymentRef through its JSON fields.
func (r DeploymentRef) String() string {
	if r.Version == "" {
		return r.Name + "@" + r.Digest
	}
	return r.Name + "@" + r.Version + "+" + r.Digest
}
