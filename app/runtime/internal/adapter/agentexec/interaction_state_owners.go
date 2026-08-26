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

func (i *interactionChildProjection) lock()   { i.mu.Lock() }
func (i *interactionChildProjection) unlock() { i.mu.Unlock() }

// interactionToolOutcomes owns the consecutive identical Tool-result invariant
// used by the doom-loop brake. It changes independently of Process topology.
type interactionToolOutcomes struct {
	mu      sync.Mutex
	key     string
	digest  [sha256.Size]byte
	repeats int
}

func (i *interactionToolOutcomes) repeated(toolName string, arguments tool.Arguments) int {
	key := toolName + "\x00" + arguments.Canonical()
	i.mu.Lock()
	defer i.mu.Unlock()
	if key != i.key {
		return 0
	}
	return i.repeats
}

func (i *interactionToolOutcomes) reset() {
	i.mu.Lock()
	i.repeats = 0
	i.mu.Unlock()
}

func (i *interactionToolOutcomes) record(
	toolName string,
	arguments tool.Arguments,
	result string,
) {
	key := toolName + "\x00" + arguments.Canonical()
	digest := sha256.Sum256([]byte(result))
	i.mu.Lock()
	defer i.mu.Unlock()
	if key == i.key && digest == i.digest {
		i.repeats++
		return
	}
	i.key = key
	i.digest = digest
	i.repeats = 1
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

func (i *interactionCommittedReplies) record(processID agent.ProcessID, message corechat.Message) {
	i.mu.Lock()
	i.byChild[processID] = message.Clone()
	i.mu.Unlock()
}

func (i *interactionCommittedReplies) lookup(processID agent.ProcessID) (corechat.Message, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	message, found := i.byChild[processID]
	return message.Clone(), found
}

func (i *interactionCommittedReplies) forget(processID agent.ProcessID) {
	i.mu.Lock()
	delete(i.byChild, processID)
	i.mu.Unlock()
}

// interactionSegmentClock owns the adapter-side start of the current product
// Segment. The Agent Process may survive several resumed Segments.
type interactionSegmentClock struct {
	mu        sync.Mutex
	startedAt time.Time
}

func (i *interactionSegmentClock) start() {
	i.mu.Lock()
	i.startedAt = time.Now().UTC()
	i.mu.Unlock()
}

func (i *interactionSegmentClock) duration(processStartedAt, finishedAt time.Time) time.Duration {
	i.mu.Lock()
	segmentStartedAt := i.startedAt
	i.mu.Unlock()
	return interactionSegmentDuration(processStartedAt, segmentStartedAt, finishedAt)
}
