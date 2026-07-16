package skills

import "errors"

var (
	// ErrNoFrontmatter means a SKILL.md did not open and close a YAML
	// frontmatter block with "---" fence lines.
	ErrNoFrontmatter = errors.New("skills: SKILL.md must open with a YAML frontmatter block delimited by ---")

	// ErrNameEmpty means the frontmatter name field was blank.
	ErrNameEmpty = errors.New("skills: name must not be empty")
	// ErrNameTooLong means the name exceeded 64 characters.
	ErrNameTooLong = errors.New("skills: name exceeds 64 characters")
	// ErrNameInvalid means the name violated the spec's character rule.
	ErrNameInvalid = errors.New("skills: name must be lowercase alphanumerics joined by single hyphens (no leading, trailing, or consecutive hyphens)")
	// ErrNameMismatch means the frontmatter name did not match the skill's
	// directory name, which the spec requires.
	ErrNameMismatch = errors.New("skills: frontmatter name must match the skill directory name")

	// ErrDescriptionEmpty means the frontmatter description field was blank.
	ErrDescriptionEmpty = errors.New("skills: description must not be empty")
	// ErrDescriptionTooLong means the description exceeded 1024 characters.
	ErrDescriptionTooLong = errors.New("skills: description exceeds 1024 characters")

	// ErrCompatibilityTooLong means the compatibility field exceeded 500
	// characters.
	ErrCompatibilityTooLong = errors.New("skills: compatibility exceeds 500 characters")

	// ErrResourcePath means a requested lexical resource path escaped its
	// skill directory or was otherwise not a valid relative path.
	ErrResourcePath = errors.New("skills: resource path escapes the skill directory")
)
