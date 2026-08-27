package skillauthoring

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/scope/app/runtime/internal/domain/skills"
)

// renderProposal encodes a domain proposal into the SKILL.md storage format. The
// YAML framing belongs beside the file store; the domain only owns the proposal
// values and lifecycle rules.
func renderProposal(proposal skills.Proposal) ([]byte, error) {
	frontmatter, err := yaml.Marshal(skillspec.Frontmatter{
		Name:        proposal.Name,
		Description: proposal.Description,
		Metadata:    proposalProvenance(proposal),
	})
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: render frontmatter: %w", err)
	}
	document := []byte("---\n" + string(frontmatter) + "---\n\n" + strings.TrimSpace(proposal.Instructions) + "\n")
	if len(document) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf(
			"skillauthoring: render proposal %q: %w: %d bytes exceeds %d",
			proposal.Name,
			skills.ErrDocumentTooLarge,
			len(document),
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	return document, nil
}

func proposalProvenance(proposal skills.Proposal) map[string]string {
	metadata := make(map[string]string, 2)
	if proposal.Origin != "" {
		metadata[metadataOrigin] = string(proposal.Origin)
	}
	if proposal.SourceSession != "" {
		metadata[metadataSourceSession] = proposal.SourceSession
	}
	if proposal.Revises {
		metadata[metadataRevises] = metadataTrue
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
