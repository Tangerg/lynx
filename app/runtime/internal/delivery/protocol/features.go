package protocol

import (
	"fmt"
	"slices"
)

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

// Feature is one entry of the published capability vocabulary: the key, plus the
// negotiation facts that belong to the FEATURE rather than to a build.
//
// Whether this composition offers a feature is a composition fact and lives with
// the composition ([FeatureCapability.Enabled]). Whether a client must ask for it,
// and whether asking changes the authoritative Run shape, are properties of the
// feature itself — §8.2 requires them to be generated from this registry rather
// than maintained beside the advertised map, because a hand-kept `clientOptIn`
// would let discovery promise a negotiation the runtime does not perform.
type Feature struct {
	Key       string
	Stability Stability
	// ClientOptIn: the runtime uses the feature only for a request whose
	// capabilities declare it, even where this build supports it.
	ClientOptIn bool
	// RequiredByRunProtocol: negotiating it changes the authoritative Run event /
	// resource shape, so a subscriber that does not understand it cannot follow the
	// Run. Exactly these keys enter a Run's frozen RunProtocolProfile.
	RequiredByRunProtocol bool
}

// features is every key discovery may advertise, in the order the canonical docs
// group them.
//
// The features map is open by design — §9 says a client treats an absent key as
// off — so this is not a closed wire enum a frame may be checked against. It is
// the published vocabulary: what a client may meaningfully ask about, and what a
// capability rule may name. [TestFeaturesAreComplete] proves it lists every
// declared constant, so a key cannot exist as a constant a rule references while
// being invisible to discovery.
var features = mustFeatures([]Feature{
	{Key: FeatureReasoning, Stability: StabilityStable},
	{Key: FeatureMultimodal, Stability: StabilityStable},
	{Key: FeatureCompaction, Stability: StabilityStable},
	{Key: FeatureTodos, Stability: StabilityStable},
	{Key: FeatureGoals, Stability: StabilityStable},
	{Key: FeatureAgentMemory, Stability: StabilityStable},
	{Key: FeatureMemory, Stability: StabilityStable},
	{Key: FeatureSkills, Stability: StabilityStable},
	{Key: FeatureMCP, Stability: StabilityStable},
	{Key: FeatureSchedules, Stability: StabilityStable},
	{Key: FeatureCodebase, Stability: StabilityStable},
	{Key: FeatureGit, Stability: StabilityStable},
	{Key: FeatureCheckpoints, Stability: StabilityStable},
	{Key: FeatureFileWatch, Stability: StabilityStable},
	{Key: FeatureLSP, Stability: StabilityStable},
	{Key: FeatureSessionExport, Stability: StabilityStable},
	{Key: FeatureRelocate, Stability: StabilityStable},
	// Subagents is the one feature that reshapes a Run's authoritative stream:
	// child runs, child lineage on every summary, and the `suspended` segment
	// outcome only exist for a Run whose profile carries it. A subscriber that does
	// not understand them cannot follow such a Run at all, which is why it is
	// opt-in AND frozen onto the Run (§8.2).
	{Key: FeatureSubagents, Stability: StabilityStable, ClientOptIn: true, RequiredByRunProtocol: true},
	// Client tools park the run on a `toolResult` interrupt the CLIENT executes,
	// so a client that did not ask for them would be handed a wait it cannot answer.
	{Key: FeatureClientTools, Stability: StabilityStable, ClientOptIn: true},
})

// Features returns a snapshot of the published capability vocabulary.
func Features() []Feature { return slices.Clone(features) }

// runProtocolFeatureValues returns the versioned subset allowed in
// RunProtocolProfile.requiredFeatures. It is derived from the same feature facts
// discovery publishes, so adding a shape-changing feature without extending the
// closed wire enum makes the enum completeness test fail.
func runProtocolFeatureValues() []string {
	out := make([]string, 0, len(features))
	for _, feature := range features {
		if feature.RequiredByRunProtocol {
			out = append(out, feature.Key)
		}
	}
	return out
}

// LookupFeature returns the published facts about a key, or false for a key this
// vocabulary does not define.
func LookupFeature(key string) (Feature, bool) {
	for _, feature := range features {
		if feature.Key == key {
			return feature, true
		}
	}
	return Feature{}, false
}

// FeatureKeys lists the vocabulary's keys in registry order, for a consumer that
// publishes the names alone.
func FeatureKeys() []string {
	keys := make([]string, 0, len(features))
	for _, feature := range features {
		keys = append(keys, feature.Key)
	}
	return keys
}

// mustFeatures rejects a vocabulary that could not be negotiated as declared.
//
// A feature that reshapes the authoritative Run stream without the client opting
// in is not an optional feature at all: the runtime would change what a Run
// publishes without anyone having asked, and every subscriber would need to
// understand it — which is the definition of stable core, not of a capability.
// §8.2 makes this a construction failure rather than a runtime check, and there is
// no request at which it could be reported: the vocabulary is wrong, not the call.
func mustFeatures(features []Feature) []Feature {
	seen := make(map[string]bool, len(features))
	for _, feature := range features {
		switch {
		case feature.Key == "":
			panic("protocol: a feature needs a key")
		case seen[feature.Key]:
			panic(fmt.Sprintf("protocol: feature %q is declared twice", feature.Key))
		case !feature.Stability.Valid():
			panic(fmt.Sprintf(
				"protocol: feature %q has invalid stability %q; expected %q or %q",
				feature.Key,
				feature.Stability,
				StabilityStable,
				StabilityExperimental,
			))
		case feature.RequiredByRunProtocol && !feature.ClientOptIn:
			panic(fmt.Sprintf("protocol: feature %q reshapes the run protocol without opt-in — it belongs in stable core, not in features", feature.Key))
		}
		seen[feature.Key] = true
	}
	return features
}
