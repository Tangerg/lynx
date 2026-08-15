package runs

import "crypto/rand"

// Resource identifiers are application-owned lifecycle identities. Their
// namespace is decided here rather than by composition or persistence.
const (
	runIDPrefix         = "run_"
	segmentIDPrefix     = "seg_"
	itemIDPrefix        = "item_"
	eventCommitIDPrefix = "event_commit_"
)

// NewRunID, NewSegmentID, and NewItemID add the application-owned namespace to an opaque
// entropy value supplied by composition. The source may be UUID, a test
// sequence, or another collision-safe generator; the use case owns the
// resulting resource shape.
func NewRunID(entropy string) string     { return runIDPrefix + entropy }
func NewSegmentID(entropy string) string { return segmentIDPrefix + entropy }
func NewItemID(entropy string) string    { return itemIDPrefix + entropy }

// newEventCommitID identifies one immutable top-level EventCommit write-set.
// Terminal identities are minted where the reducer creates the write-set and
// retained across retries. Non-terminal publication mints its identity at the
// transaction boundary because it is never retried outside that call.
func newEventCommitID() string { return eventCommitIDPrefix + rand.Text() }
