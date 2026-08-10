// Package promptqueue owns follow-up messages waiting behind a running turn.
// It is application state rather than terminal state: views receive detached
// snapshots, while runtime adapters continue to see ordinary StartRun calls.
package promptqueue

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

var (
	ErrEntryNotFound     = errors.New("prompt queue: entry not found")
	ErrEntryHeld         = errors.New("prompt queue: entry is already held")
	ErrMoveUnavailable   = errors.New("prompt queue: move unavailable")
	ErrSessionIDRequired = errors.New("prompt queue: session id is empty")
)

// Entry is one immutable queue projection. Message owns a detached attachment
// slice, so callers cannot mutate the queue through a snapshot.
type Entry struct {
	ID        uint64
	SessionID string
	Message   agent.Message
	Held      bool
}

// Snapshot is one session's detached FIFO view.
type Snapshot struct {
	Entries  []Entry
	Revision uint64
}

// Queue keeps independent FIFO sequences for every session visited by the app.
// This prevents a failed follow-up from moving to another session when the
// user switches away and later comes back.
type Queue struct {
	mu       sync.RWMutex
	nextID   uint64
	revision uint64
	entries  map[string][]Entry
}

func New() *Queue {
	return &Queue{entries: make(map[string][]Entry)}
}

func (q *Queue) Enqueue(sessionID string, message agent.Message) (Entry, error) {
	if err := validateEntry(sessionID, message); err != nil {
		return Entry{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureEntries()
	q.nextID++
	entry := Entry{ID: q.nextID, SessionID: strings.Clone(sessionID), Message: message.Clone()}
	q.entries[sessionID] = append(q.entries[sessionID], entry)
	q.revision++
	return cloneEntry(entry), nil
}

func (q *Queue) Snapshot(sessionID string) Snapshot {
	q.mu.RLock()
	defer q.mu.RUnlock()
	entries := q.entries[sessionID]
	out := make([]Entry, len(entries))
	for index, entry := range entries {
		out[index] = cloneEntry(entry)
	}
	return Snapshot{Entries: out, Revision: q.revision}
}

func (q *Queue) Next(sessionID string) (Entry, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	entries := q.entries[sessionID]
	if len(entries) == 0 || entries[0].Held {
		return Entry{}, false
	}
	return cloneEntry(entries[0]), true
}

// Hold prevents the FIFO consumer from dispatching an entry while an editor
// owns it. A held entry remains visible and keeps its position in the queue.
func (q *Queue) Hold(sessionID string, id uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if entries[index].Held {
		return ErrEntryHeld
	}
	entries[index].Held = true
	q.entries[sessionID] = entries
	q.revision++
	return nil
}

// Release makes a held entry dispatchable again. It is idempotent so dialog
// teardown can safely release ownership after any close path.
func (q *Queue) Release(sessionID string, id uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if !entries[index].Held {
		return nil
	}
	entries[index].Held = false
	q.entries[sessionID] = entries
	q.revision++
	return nil
}

func (q *Queue) Update(sessionID string, id uint64, message agent.Message) error {
	if err := validateEntry(sessionID, message); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	index := entryIndex(q.entries[sessionID], id)
	if index < 0 {
		return ErrEntryNotFound
	}
	q.entries[sessionID][index].Message = message.Clone()
	q.revision++
	return nil
}

func (q *Queue) Remove(sessionID string, id uint64) (Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return Entry{}, ErrEntryNotFound
	}
	removed := cloneEntry(entries[index])
	clear(entries[index : index+1])
	entries = slices.Delete(entries, index, index+1)
	if len(entries) == 0 {
		delete(q.entries, sessionID)
	} else {
		q.entries[sessionID] = entries
	}
	q.revision++
	return removed, nil
}

func (q *Queue) Move(sessionID string, id uint64, offset int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	from := entryIndex(entries, id)
	if from < 0 {
		return ErrEntryNotFound
	}
	to := from + offset
	if offset == 0 || to < 0 || to >= len(entries) {
		return ErrMoveUnavailable
	}
	entry := entries[from]
	entries = slices.Delete(entries, from, from+1)
	entries = slices.Insert(entries, to, entry)
	q.entries[sessionID] = entries
	q.revision++
	return nil
}

// Promote moves an entry to the front without changing its stable identity.
// It is the queue-level half of "send now": orchestration decides whether a
// running turn must be canceled before the promoted entry can be dispatched.
func (q *Queue) Promote(sessionID string, id uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	from := entryIndex(entries, id)
	if from < 0 {
		return ErrEntryNotFound
	}
	if from == 0 {
		return nil
	}
	entry := entries[from]
	entries = slices.Delete(entries, from, from+1)
	entries = slices.Insert(entries, 0, entry)
	q.entries[sessionID] = entries
	q.revision++
	return nil
}

func (q *Queue) Clear(sessionID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	if len(entries) == 0 {
		return 0
	}
	count := len(entries)
	clear(entries)
	delete(q.entries, sessionID)
	q.revision++
	return count
}

func (q *Queue) ensureEntries() {
	if q.entries == nil {
		q.entries = make(map[string][]Entry)
	}
}

func validateEntry(sessionID string, message agent.Message) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	return nil
}

func entryIndex(entries []Entry, id uint64) int {
	return slices.IndexFunc(entries, func(entry Entry) bool { return entry.ID == id })
}

func cloneEntry(entry Entry) Entry {
	entry.SessionID = strings.Clone(entry.SessionID)
	entry.Message = entry.Message.Clone()
	return entry
}
