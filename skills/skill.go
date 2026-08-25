package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || lines[0] != "---" {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, ErrNoFrontmatter)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, ErrNoFrontmatter)
	}

	var frontmatter Frontmatter
	block := strings.Join(lines[1:end], "\n")
	if err := yaml.Unmarshal([]byte(block), &frontmatter); err != nil {
		return nil, fmt.Errorf("%w: parse frontmatter: %w", ErrInvalidSkill, err)
	}

	skill := &Skill{
		Frontmatter:  frontmatter,
		Instructions: strings.TrimSpace(strings.Join(lines[end+1:], "\n")),
	}
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSkill, err)
	}
	return skill, nil
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
