package skills

// Skill is a fully loaded skill: its frontmatter metadata plus the Markdown
// instruction body. Bundled resource files (references/, assets/, scripts/)
// are not loaded here — they are opened on demand via [ResourceSource],
// the third level of progressive disclosure.
type Skill struct {
	Frontmatter
	Body string
}

// Summary is the metadata view — just enough for an agent to decide whether a
// skill is relevant without loading its instructions (progressive-disclosure
// level 1).
type Summary struct {
	Name        string
	Description string
}

func (s Skill) Summary() Summary {
	return Summary{Name: s.Name, Description: s.Description}
}
