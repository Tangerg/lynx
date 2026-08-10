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

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

var (
	ErrEntryNotFound     = errors.New("prompt queue: entry not found")
	ErrMoveUnavailable   = errors.New("prompt queue: move unavailable")
	ErrSessionIDRequired = errors.New("prompt queue: session id is empty")
)

// Entry is one immutable queue projection. Message owns a detached attachment
// slice, so callers cannot mutate the store through a snapshot.
type Entry struct {
	ID        uint64
	SessionID string
	Message   client.Message
}

// Snapshot is one session's detached FIFO view.
type Snapshot struct {
	Entries  []Entry
	Revision uint64
}

// Store keeps independent FIFO queues for every session visited by the app.
// This prevents a failed follow-up from moving to another session when the
// user switches away and later comes back.
type Store struct {
	mu       sync.RWMutex
	nextID   uint64
	revision uint64
	entries  map[string][]Entry
}

func New() *Store {
	return &Store{entries: make(map[string][]Entry)}
}

func (s *Store) Enqueue(sessionID string, message client.Message) (Entry, error) {
	if err := validate(sessionID, message); err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.nextID++
	entry := Entry{ID: s.nextID, SessionID: strings.Clone(sessionID), Message: cloneMessage(message)}
	s.entries[sessionID] = append(s.entries[sessionID], entry)
	s.revision++
	return cloneEntry(entry), nil
}

func (s *Store) Snapshot(sessionID string) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.entries[sessionID]
	out := make([]Entry, len(entries))
	for index, entry := range entries {
		out[index] = cloneEntry(entry)
	}
	return Snapshot{Entries: out, Revision: s.revision}
}

func (s *Store) Next(sessionID string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.entries[sessionID]
	if len(entries) == 0 {
		return Entry{}, false
	}
	return cloneEntry(entries[0]), true
}

func (s *Store) Update(sessionID string, id uint64, message client.Message) error {
	if err := validate(sessionID, message); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := find(s.entries[sessionID], id)
	if index < 0 {
		return ErrEntryNotFound
	}
	s.entries[sessionID][index].Message = cloneMessage(message)
	s.revision++
	return nil
}

func (s *Store) Remove(sessionID string, id uint64) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.entries[sessionID]
	index := find(entries, id)
	if index < 0 {
		return Entry{}, ErrEntryNotFound
	}
	removed := cloneEntry(entries[index])
	clear(entries[index : index+1])
	entries = slices.Delete(entries, index, index+1)
	if len(entries) == 0 {
		delete(s.entries, sessionID)
	} else {
		s.entries[sessionID] = entries
	}
	s.revision++
	return removed, nil
}

func (s *Store) Move(sessionID string, id uint64, offset int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.entries[sessionID]
	from := find(entries, id)
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
	s.entries[sessionID] = entries
	s.revision++
	return nil
}

func (s *Store) Clear(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.entries[sessionID]
	if len(entries) == 0 {
		return 0
	}
	count := len(entries)
	clear(entries)
	delete(s.entries, sessionID)
	s.revision++
	return count
}

func (s *Store) ensure() {
	if s.entries == nil {
		s.entries = make(map[string][]Entry)
	}
}

func validate(sessionID string, message client.Message) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	if err := message.Validate(); err != nil {
		return fmt.Errorf("prompt queue: %w", err)
	}
	return nil
}

func find(entries []Entry, id uint64) int {
	return slices.IndexFunc(entries, func(entry Entry) bool { return entry.ID == id })
}

func cloneEntry(entry Entry) Entry {
	entry.SessionID = strings.Clone(entry.SessionID)
	entry.Message = cloneMessage(entry.Message)
	return entry
}

func cloneMessage(message client.Message) client.Message {
	message.Text = strings.Clone(message.Text)
	message.Attachments = slices.Clone(message.Attachments)
	return message
}
