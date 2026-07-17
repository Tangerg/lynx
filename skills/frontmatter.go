package skills

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxNameLen          = 64
	maxDescriptionLen   = 1024
	maxCompatibilityLen = 500
)

// nameRE encodes the spec's name rule: lowercase alphanumerics joined by
// single hyphens — no leading, trailing, or consecutive hyphens.
var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Frontmatter is the YAML metadata block at the head of a SKILL.md file, as
// defined by the Agent Skills specification.
type Frontmatter struct {
	// Name is the unique skill identifier; it must match the skill's parent
	// directory name. Required.
	Name string `yaml:"name"`
	// Description states what the skill does and when to use it — the text an
	// agent reads to decide relevance. Required.
	Description string `yaml:"description"`
	// License names the license, or a bundled license file. Optional.
	License string `yaml:"license,omitempty"`
	// Compatibility states environment requirements (target product, system
	// packages, network access, ...). Optional.
	Compatibility string `yaml:"compatibility,omitempty"`
	// Metadata is an arbitrary string map for client-defined properties.
	Metadata map[string]string `yaml:"metadata,omitempty"`
	// AllowedTools is a space-separated list of pre-approved tools. Optional
	// and experimental; this package parses but does not enforce it.
	AllowedTools string `yaml:"allowed-tools,omitempty"`
}

// AllowedToolList splits the space-separated allowed-tools field into its
// entries. The field is experimental and advisory — this package neither
// interprets nor enforces it; the splitter is offered for callers that do.
func (f Frontmatter) AllowedToolList() []string {
	return strings.Fields(f.AllowedTools)
}

// Validate reports whether the frontmatter satisfies the spec's constraints,
// joining every violation so a caller sees them all at once.
func (f Frontmatter) Validate() error {
	var errs []error

	if err := validateName(f.Name); err != nil {
		errs = append(errs, err)
	}

	// Description / Compatibility limits are in characters (the spec's
	// unit), so count runes — byte length over-counts non-ASCII text.
	// Name stays byte-counted: its regex locks it to ASCII anyway.
	descriptionLen := utf8.RuneCountInString(f.Description)
	switch {
	case strings.TrimSpace(f.Description) == "":
		errs = append(errs, ErrDescriptionEmpty)
	case descriptionLen > maxDescriptionLen:
		errs = append(errs, fmt.Errorf("%w: %d characters", ErrDescriptionTooLong, descriptionLen))
	}

	if compatibilityLen := utf8.RuneCountInString(f.Compatibility); compatibilityLen > maxCompatibilityLen {
		errs = append(errs, fmt.Errorf("%w: %d characters", ErrCompatibilityTooLong, compatibilityLen))
	}

	return errors.Join(errs...)
}

// validateName is the single owner of the specification's name rules. Source
// implementations call it before touching a filesystem; Frontmatter.Validate
// calls it for loaded metadata.
func validateName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return ErrNameEmpty
	case len(name) > maxNameLen:
		return fmt.Errorf("%w: %d characters", ErrNameTooLong, len(name))
	case !nameRE.MatchString(name):
		return fmt.Errorf("%w: %q", ErrNameInvalid, name)
	default:
		return nil
	}
}

// Parse splits a SKILL.md document into its YAML frontmatter and Markdown
// body. The document must open with a "---" line, hold a YAML block, and
// close it with another "---" line; everything after the closing line is the
// trimmed body. Parse does not validate the frontmatter — call
// [Frontmatter.Validate] for that.
func Parse(content []byte) (Frontmatter, string, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || lines[0] != "---" {
		return Frontmatter{}, "", ErrNoFrontmatter
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Frontmatter{}, "", ErrNoFrontmatter
	}

	var fm Frontmatter
	block := strings.Join(lines[1:end], "\n")
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return Frontmatter{}, "", fmt.Errorf("skills: parse frontmatter: %w", err)
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return fm, body, nil
}
