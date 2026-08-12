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
	CommandID agent.CommandID
	SessionID string
	Message   agent.Message
	Options   agent.RunOptions
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
	commandID, err := agent.NewCommandID()
	if err != nil {
		return Entry{}, fmt.Errorf("prompt queue: %w", err)
	}
	return q.EnqueueCommand(commandID, sessionID, message, agent.RunOptions{})
}

// EnqueueCommand preserves a mutation identity already allocated by the
// authoring transaction. Queue edits allocate a new identity because changing
// content creates a different runtime operation fingerprint.
func (q *Queue) EnqueueCommand(commandID agent.CommandID, sessionID string, message agent.Message, options agent.RunOptions) (Entry, error) {
	if err := validateEntry(commandID, sessionID, message); err != nil {
		return Entry{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureEntries()
	q.nextID++
	entry := Entry{
		ID: q.nextID, CommandID: commandID, SessionID: strings.Clone(sessionID),
		Message: message.Clone(), Options: options.Clone(),
	}
	q.entries[sessionID] = append(q.entries[sessionID], entry)
	q.revision++
	return cloneEntry(entry), nil
}

// Restore replaces one session queue from a durable authoring outbox. Command
// identity and order are preserved so runtime retries remain idempotent.
func (q *Queue) Restore(sessionID string, commands []agent.StartRun) error {
	entries := make([]Entry, len(commands))
	seen := make(map[agent.CommandID]struct{}, len(commands))
	for index, command := range commands {
		if command.SessionID != sessionID {
			return fmt.Errorf("prompt queue: command %d belongs to session %s", index+1, command.SessionID)
		}
		if err := validateEntry(command.CommandID, sessionID, command.Message); err != nil {
			return err
		}
		if _, duplicate := seen[command.CommandID]; duplicate {
			return fmt.Errorf("prompt queue: command %s is duplicated", command.CommandID)
		}
		seen[command.CommandID] = struct{}{}
		entries[index] = Entry{
			CommandID: command.CommandID, SessionID: sessionID,
			Message: command.Message.Clone(), Options: command.Options.Clone(),
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureEntries()
	for index := range entries {
		q.nextID++
		entries[index].ID = q.nextID
	}
	if len(entries) == 0 {
		delete(q.entries, sessionID)
	} else {
		q.entries[sessionID] = entries
	}
	q.revision++
	return nil
}

// RestoreSnapshot replaces one session with a previously observed queue
// snapshot. It is intended for rolling back an in-memory mutation when the
// corresponding durable authoring transaction cannot be committed.
func (q *Queue) RestoreSnapshot(sessionID string, snapshot Snapshot) error {
	entries := make([]Entry, len(snapshot.Entries))
	seenIDs := make(map[uint64]struct{}, len(snapshot.Entries))
	seenCommands := make(map[agent.CommandID]struct{}, len(snapshot.Entries))
	var maximumID uint64
	for index, entry := range snapshot.Entries {
		if entry.SessionID != sessionID {
			return fmt.Errorf("prompt queue: entry %d belongs to session %s", index+1, entry.SessionID)
		}
		if entry.ID == 0 {
			return fmt.Errorf("prompt queue: entry %d has no identity", index+1)
		}
		if err := validateEntry(entry.CommandID, sessionID, entry.Message); err != nil {
			return err
		}
		if _, duplicate := seenIDs[entry.ID]; duplicate {
			return fmt.Errorf("prompt queue: entry %d is duplicated", entry.ID)
		}
		if _, duplicate := seenCommands[entry.CommandID]; duplicate {
			return fmt.Errorf("prompt queue: command %s is duplicated", entry.CommandID)
		}
		seenIDs[entry.ID] = struct{}{}
		seenCommands[entry.CommandID] = struct{}{}
		maximumID = max(maximumID, entry.ID)
		entries[index] = cloneEntry(entry)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureEntries()
	q.nextID = max(q.nextID, maximumID)
	if len(entries) == 0 {
		delete(q.entries, sessionID)
	} else {
		q.entries[sessionID] = entries
	}
	q.revision++
	return nil
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
	commandID, err := agent.NewCommandID()
	if err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	if err := validateEntry(commandID, sessionID, message); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	index := entryIndex(q.entries[sessionID], id)
	if index < 0 {
		return ErrEntryNotFound
	}
	q.entries[sessionID][index].Message = message.Clone()
	q.entries[sessionID][index].CommandID = commandID
	q.revision++
	return nil
}

// ReplaceCommandID gives a queued intent a fresh runtime mutation identity
// after the previous identity received a definitive refusal. Message content,
// FIFO position, edit ownership, and local entry identity remain unchanged.
func (q *Queue) ReplaceCommandID(sessionID string, id uint64, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := q.entries[sessionID]
	index := entryIndex(entries, id)
	if index < 0 {
		return ErrEntryNotFound
	}
	if slices.ContainsFunc(entries, func(entry Entry) bool {
		return entry.ID != id && entry.CommandID == commandID
	}) {
		return fmt.Errorf("prompt queue: command %s is duplicated", commandID)
	}
	if entries[index].CommandID == commandID {
		return nil
	}
	entries[index].CommandID = commandID
	q.entries[sessionID] = entries
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

func validateEntry(commandID agent.CommandID, sessionID string, message agent.Message) error {
	if err := commandID.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
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
	entry.Options = entry.Options.Clone()
	return entry
}
