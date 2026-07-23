package runs

// Resource identifiers are application-owned lifecycle identities. Delivery
// may frame them for the wire, but neither Bootstrap nor a persistence adapter
// decides their namespace.
const (
	runIDPrefix     = "run_"
	segmentIDPrefix = "seg_"
	itemIDPrefix    = "item_"
)

// NewRunID and NewSegmentID add the application-owned namespace to an opaque
// entropy value supplied by composition. The source may be UUID, a test
// sequence, or another collision-safe generator; the use case owns the
// resulting resource shape.
func NewRunID(entropy string) string     { return runIDPrefix + entropy }
func NewSegmentID(entropy string) string { return segmentIDPrefix + entropy }
