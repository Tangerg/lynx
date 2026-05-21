package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

// FileMessageStore persists [chat.Message] history one JSONL file
// per conversation under <home>/messages. Implements [memory.Store]
// so it slots into the lynx-core chat-memory middleware exactly
// like [memory.InMemoryStore].
//
// JSONL was chosen over a single JSON array so:
//   - appends stay O(1) regardless of history size
//   - partial corruption (last line truncated) doesn't destroy older
//     turns — Read skips unparseable lines with a warning
//   - tail-friendly: `tail -f messages/<id>.jsonl` shows live turns
//
// Trade-offs vs [FileSessionService]: session count is small (<100
// typical) so atomic rewrite is fine. Message count grows
// unboundedly per session, so append-only is the right pattern.
type FileMessageStore struct {
	dir string

	// mu guards the concurrent-conversation maps. Per-conversation
	// IO still uses the OS file lock implicitly through O_APPEND
	// + the per-conversation Mutex obtained from locks.
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewFileMessageStore opens <home>/messages and returns a ready
// store. The directory is created lazily; ID-to-filename mapping
// uses a strict-character allowlist so a stray ".." in a session
// id can't escape the directory.
func NewFileMessageStore() (*FileMessageStore, error) {
	dir, err := SubDir("messages")
	if err != nil {
		return nil, err
	}
	return &FileMessageStore{dir: dir, locks: map[string]*sync.Mutex{}}, nil
}

var _ memory.Store = (*FileMessageStore)(nil)

// pathFor returns the JSONL file for conversationID. Rejects ids
// containing path separators or "..".
func (s *FileMessageStore) pathFor(id string) (string, error) {
	if id == "" || id == "." || id == ".." {
		return "", fmt.Errorf("storage: invalid conversation id %q", id)
	}
	for _, r := range id {
		if r == '/' || r == '\\' || r == 0 {
			return "", fmt.Errorf("storage: invalid conversation id %q", id)
		}
	}
	return filepath.Join(s.dir, id+".jsonl"), nil
}

// lockFor returns a per-conversation mutex, allocating one on first
// use. Different conversations write concurrently; same conversation
// serializes.
func (s *FileMessageStore) lockFor(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[id]
	if !ok {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	return m
}

// Read returns every message stored for conversationID, preserving
// write order. Missing file → empty slice (matches
// [memory.InMemoryStore]).
func (s *FileMessageStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.pathFor(conversationID)
	if err != nil {
		return nil, err
	}
	lock := s.lockFor(conversationID)
	lock.Lock()
	defer lock.Unlock()

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []chat.Message{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", path, err)
	}
	defer f.Close()

	var out []chat.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		msg, err := chat.UnmarshalMessage(line)
		if err != nil {
			// Skip malformed entries rather than failing the read —
			// keeps a single corrupt write from poisoning the whole
			// conversation.
			continue
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("storage: scan %q: %w", path, err)
	}
	return out, nil
}

// Write appends every message to the conversation's file. No-op for
// empty messages.
func (s *FileMessageStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	path, err := s.pathFor(conversationID)
	if err != nil {
		return err
	}
	lock := s.lockFor(conversationID)
	lock.Lock()
	defer lock.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("storage: open %q: %w", path, err)
	}
	defer f.Close()

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("storage: marshal message: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("storage: append: %w", err)
		}
	}
	return nil
}

// Clear deletes the conversation's file. Idempotent — unknown id
// is not an error (matches [memory.InMemoryStore]).
func (s *FileMessageStore) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathFor(conversationID)
	if err != nil {
		return err
	}
	lock := s.lockFor(conversationID)
	lock.Lock()
	defer lock.Unlock()

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: remove %q: %w", path, err)
	}
	return nil
}
