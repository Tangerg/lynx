package protocol

// The capability keys of `capabilities.features` (API.md §9).
//
// They are wire values — a client reads them from discovery and gates its UI on
// them — so they belong here, with the shapes that carry them, and not in
// whichever package happened to mention one first. Before this they had three
// authors: the capability rules that gate a method, the map discovery advertises,
// and the frontend's own union of the same nineteen names. Three spellings of one
// vocabulary is three chances for a typo to read as "this build cannot do that".
const (
	FeatureReasoning     = "reasoning"
	FeatureMultimodal    = "multimodal"
	FeatureCompaction    = "compaction"
	FeatureTodos         = "todos"
	FeatureGoals         = "goals"
	FeatureAgentMemory   = "agentMemory"
	FeatureMemory        = "memory"
	FeatureSkills        = "skills"
	FeatureMCP           = "mcp"
	FeatureSchedules     = "schedules"
	FeatureCodebase      = "codebase"
	FeatureGit           = "git"
	FeatureCheckpoints   = "checkpoints"
	FeatureFileWatch     = "fileWatch"
	FeatureLSP           = "lsp"
	FeatureSessionExport = "sessionExport"
	FeatureRelocate      = "relocate"
	FeatureSubagents     = "subagents"
	FeatureClientTools   = "clientTools"
)

// Features is every key discovery may advertise, in the order the canonical docs
// group them.
//
// The features map is open by design — §9 says a client treats an absent key as
// off — so this is not a closed wire enum a frame may be checked against. It is
// the published vocabulary: what a client may meaningfully ask about, and what a
// capability rule may name. The table references the constants rather than
// repeating the literals, and [TestFeaturesAreComplete] proves it lists all of
// them.
var Features = []string{
	FeatureReasoning,
	FeatureMultimodal,
	FeatureCompaction,
	FeatureTodos,
	FeatureGoals,
	FeatureAgentMemory,
	FeatureMemory,
	FeatureSkills,
	FeatureMCP,
	FeatureSchedules,
	FeatureCodebase,
	FeatureGit,
	FeatureCheckpoints,
	FeatureFileWatch,
	FeatureLSP,
	FeatureSessionExport,
	FeatureRelocate,
	FeatureSubagents,
	FeatureClientTools,
}
