package capabilityflow

const userSkillLibraryLane = "library\x00user"

func skillProposalLane(workspace, name string) string {
	return "proposal\x00" + workspace + "\x00" + name
}
