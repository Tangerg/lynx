package skillauthoring

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// renderDraft encodes a domain proposal into the SKILL.md storage format. The
// YAML framing belongs beside the file store; the domain only owns the proposal
// values and lifecycle rules.
func renderDraft(draft skills.Draft) ([]byte, error) {
	frontmatter, err := yaml.Marshal(skillspec.Frontmatter{
		Name:        draft.Name,
		Description: draft.Description,
		Metadata:    draftProvenance(draft),
	})
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: render frontmatter: %w", err)
	}
	return []byte("---\n" + string(frontmatter) + "---\n\n" + strings.TrimSpace(draft.Body) + "\n"), nil
}

func draftProvenance(draft skills.Draft) map[string]string {
	metadata := make(map[string]string, 2)
	if draft.CreatedBy != "" {
		metadata[skills.MetadataCreatedBy] = draft.CreatedBy
	}
	if draft.SourceSession != "" {
		metadata[skills.MetadataSourceSession] = draft.SourceSession
	}
	if draft.Revises {
		metadata[skills.MetadataRevises] = skills.MetadataTrue
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
