package protocol

// SkillDraft is one agent-mined skill proposal awaiting offline review
// (skills.drafts.list). Name+Revision form the content-addressed handle a
// promote/reject call carries. CreatedBy/SourceSession are the provenance read
// from the draft's frontmatter (created_by is "agent" for a mined draft).
type SkillDraft struct {
	Name          string `json:"name"`
	Revision      string `json:"revision"`
	Description   string `json:"description,omitempty"`
	CreatedBy     string `json:"createdBy,omitempty"`
	SourceSession string `json:"sourceSession,omitempty"`
}

// SkillDraftRef identifies the exact staged draft a skills.drafts.promote /
// reject call acts on. Both fields are required; Revision binds the name to the
// immutable bytes so a decision cannot act on a different revision.
type SkillDraftRef struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}
