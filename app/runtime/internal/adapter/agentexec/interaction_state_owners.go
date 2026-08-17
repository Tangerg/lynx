package agentexec

import (
	"crypto/sha256"
	"sync"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// interactionChildProjection serializes projection of completed Delegate
// children with installation of a committed waiting-subtree replacement.
type interactionChildProjection struct{ mu sync.Mutex }

func (projection *interactionChildProjection) lock()   { projection.mu.Lock() }
func (projection *interactionChildProjection) unlock() { projection.mu.Unlock() }

// interactionToolOutcomes owns the consecutive identical Tool-result invariant
// used by the doom-loop brake. It changes independently of Process topology.
type interactionToolOutcomes struct {
	mu      sync.Mutex
	key     string
	digest  [sha256.Size]byte
	repeats int
}

func (outcomes *interactionToolOutcomes) repeated(toolName string, arguments tool.Arguments) int {
	key := toolName + "\x00" + arguments.Canonical()
	outcomes.mu.Lock()
	defer outcomes.mu.Unlock()
	if key != outcomes.key {
		return 0
	}
	return outcomes.repeats
}

func (outcomes *interactionToolOutcomes) reset() {
	outcomes.mu.Lock()
	outcomes.repeats = 0
	outcomes.mu.Unlock()
}

func (outcomes *interactionToolOutcomes) record(
	toolName string,
	arguments tool.Arguments,
	result string,
) {
	key := toolName + "\x00" + arguments.Canonical()
	digest := sha256.Sum256([]byte(result))
	outcomes.mu.Lock()
	defer outcomes.mu.Unlock()
	if key == outcomes.key && digest == outcomes.digest {
		outcomes.repeats++
		return
	}
	outcomes.key = key
	outcomes.digest = digest
	outcomes.repeats = 1
}

// interactionCommittedReplies owns assistant values already accepted by the
// authoritative Run projection until the corresponding Delegate closes.
type interactionCommittedReplies struct {
	mu      sync.Mutex
	byChild map[agent.ProcessID]corechat.Message
}

func newInteractionCommittedReplies() interactionCommittedReplies {
	return interactionCommittedReplies{byChild: make(map[agent.ProcessID]corechat.Message)}
}

func (replies *interactionCommittedReplies) record(processID agent.ProcessID, message corechat.Message) {
	replies.mu.Lock()
	replies.byChild[processID] = message.Clone()
	replies.mu.Unlock()
}

func (replies *interactionCommittedReplies) lookup(processID agent.ProcessID) (corechat.Message, bool) {
	replies.mu.Lock()
	defer replies.mu.Unlock()
	message, found := replies.byChild[processID]
	return message.Clone(), found
}

func (replies *interactionCommittedReplies) forget(processID agent.ProcessID) {
	replies.mu.Lock()
	delete(replies.byChild, processID)
	replies.mu.Unlock()
}

// interactionSegmentClock owns the adapter-side start of the current product
// Segment. The Agent Process may survive several resumed Segments.
type interactionSegmentClock struct {
	mu        sync.Mutex
	startedAt time.Time
}

func (clock *interactionSegmentClock) start() {
	clock.mu.Lock()
	clock.startedAt = time.Now().UTC()
	clock.mu.Unlock()
}

func (clock *interactionSegmentClock) duration(processStartedAt, finishedAt time.Time) time.Duration {
	clock.mu.Lock()
	segmentStartedAt := clock.startedAt
	clock.mu.Unlock()
	return interactionSegmentDuration(processStartedAt, segmentStartedAt, finishedAt)
}
