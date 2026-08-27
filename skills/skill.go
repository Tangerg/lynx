package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	documentfrontmatter "github.com/adrg/frontmatter"
	"go.yaml.in/yaml/v4"
)

const frontmatterFence = "---"

var yamlFrontmatterFormat = documentfrontmatter.NewFormat(frontmatterFence, frontmatterFence, yaml.Unmarshal)

// Skill is a fully loaded skill: its frontmatter metadata plus the Markdown
// instruction body. Bundled resource files (references/, assets/, scripts/)
// are not loaded here — they are opened on demand via [ResourceSource],
// the third level of progressive disclosure.
type Skill struct {
	Frontmatter
	Instructions string
}

// Parse constructs and validates a skill from a complete SKILL.md document.
// The document must open with a "---" line, contain YAML frontmatter, and
// close the block with another "---" line. Everything after the closing
// fence is the skill's Markdown instructions.
func Parse(content []byte) (*Skill, error) {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	normalized = bytes.TrimPrefix(normalized, []byte("\ufeff"))
	if !bytes.HasPrefix(normalized, []byte(frontmatterFence+"\n")) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, ErrNoFrontmatter)
	}

	var metadata Frontmatter
	body, err := documentfrontmatter.MustParse(bytes.NewReader(normalized), &metadata, yamlFrontmatterFormat)
	if errors.Is(err, documentfrontmatter.ErrNotFound) || errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, ErrNoFrontmatter)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: parse frontmatter: %w", ErrInvalidSkill, err)
	}
	consumed := normalized[:len(normalized)-len(body)]
	if !hasExactClosingFence(consumed) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, ErrNoFrontmatter)
	}

	skill := &Skill{
		Frontmatter:  metadata,
		Instructions: strings.TrimSpace(string(body)),
	}
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, err)
	}
	return skill, nil
}

func hasExactClosingFence(consumed []byte) bool {
	consumed = bytes.TrimSuffix(consumed, []byte("\n"))
	lineStart := bytes.LastIndexByte(consumed, '\n') + 1
	return bytes.Equal(consumed[lineStart:], []byte(frontmatterFence))
}

// Validate reports whether the skill is a valid in-memory Agent Skill.
func (s *Skill) Validate() error {
	if s == nil {
		return ErrNilSkill
	}
	return s.Frontmatter.Validate()
}

// Summary is the metadata view — just enough for an agent to decide whether a
// skill is relevant without loading its instructions (progressive-disclosure
// level 1).
type Summary struct {
	Name        string
	Description string
}

// Validate reports whether the summary is a valid progressive-disclosure
// view of a skill.
func (s Summary) Validate() error {
	return (Frontmatter{Name: s.Name, Description: s.Description}).Validate()
}

func (s Skill) Summary() Summary {
	return Summary{Name: s.Name, Description: s.Description}
}
