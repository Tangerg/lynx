package protocol

// SkillProposalOrigin identifies why the runtime submitted a proposal.
type SkillProposalOrigin string

const (
	SkillProposalOriginRequested SkillProposalOrigin = "requested"
	SkillProposalOriginMined     SkillProposalOrigin = "mined"
)

// SkillProposal is complete immutable Skill content awaiting review. Name,
// Revision, and Scope form the content-addressed reference used by approve and
// reject operations.
type SkillProposal struct {
	Name          string              `json:"name"`
	Revision      string              `json:"revision"`
	Scope         SkillScope          `json:"scope"`
	Description   string              `json:"description"`
	Instructions  string              `json:"instructions"`
	Origin        SkillProposalOrigin `json:"origin,omitempty"`
	SourceSession string              `json:"sourceSession,omitempty"`
	Revises       bool                `json:"revises,omitempty"`
}

// SkillProposalRef identifies the exact proposal and workspace review context
// that an approve or reject operation acts on.
type SkillProposalRef struct {
	Workspace WorkspaceRef `json:"workspace"`
	Name      string       `json:"name"`
	Revision  string       `json:"revision"`
	Scope     SkillScope   `json:"scope"`
}
