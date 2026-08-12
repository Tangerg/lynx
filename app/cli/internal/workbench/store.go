// Package workbench owns durable, CLI-local authoring state. It deliberately
// knows nothing about terminal widgets or runtime persistence.
package workbench

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	formatVersion       = 1
	defaultHistoryLimit = 1000
	defaultStashLimit   = 100
	defaultWorkspaceCap = 50
)

// Config controls bounded state and supplies deterministic identity sources to
// tests. Zero values select production defaults.
type Config struct {
	HistoryLimit   int
	StashLimit     int
	WorkspaceLimit int
	Now            func() time.Time
	Random         io.Reader
}

// Stash is an explicitly named prompt snapshot.
type Stash struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"createdAt"`
	Message   agent.Message `json:"message"`
}

// Workspace is one recently used authoring root.
type Workspace struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// Store is the aggregate root for CLI authoring state. Every mutating method
// updates memory only after its durable replacement succeeds.
type Store struct {
	mu             sync.Mutex
	directory      string
	historyLimit   int
	stashLimit     int
	workspaceLimit int
	now            func() time.Time
	random         io.Reader
	history        []agent.Message
	drafts         map[string]agent.Message
	stashes        []Stash
	workspaces     []Workspace
}

// Open loads a file-backed store. An empty directory creates a memory-only
// store, which keeps embedders and in-memory tests explicit.
func Open(directory string, config Config) (*Store, error) {
	store := &Store{
		directory:      strings.TrimSpace(directory),
		historyLimit:   positiveOr(config.HistoryLimit, defaultHistoryLimit),
		stashLimit:     positiveOr(config.StashLimit, defaultStashLimit),
		workspaceLimit: positiveOr(config.WorkspaceLimit, defaultWorkspaceCap),
		now:            config.Now,
		random:         config.Random,
		drafts:         make(map[string]agent.Message),
	}
	if store.now == nil {
		store.now = time.Now
	}
	if store.random == nil {
		store.random = rand.Reader
	}
	if store.directory == "" {
		return store, nil
	}
	if !filepath.IsAbs(store.directory) {
		return nil, errors.New("workbench directory must be absolute")
	}
	store.directory = filepath.Clean(store.directory)
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return nil, fmt.Errorf("create workbench directory: %w", err)
	}
	if err := store.loadOptional("history.json", &store.history); err != nil {
		return nil, fmt.Errorf("load prompt history: %w", err)
	}
	if err := store.loadOptional("stashes.json", &store.stashes); err != nil {
		return nil, fmt.Errorf("load prompt stashes: %w", err)
	}
	if err := store.loadOptional("workspaces.json", &store.workspaces); err != nil {
		return nil, fmt.Errorf("load recent workspaces: %w", err)
	}
	store.history = tailMessages(store.history, store.historyLimit)
	store.stashes = tailStashes(store.stashes, store.stashLimit)
	store.workspaces = slices.Clone(store.workspaces[:min(len(store.workspaces), store.workspaceLimit)])
	return store, nil
}

// History returns detached prompts in oldest-to-newest order.
func (s *Store) History() []agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMessages(s.history)
}

// Remember records a submitted or deliberately cleared prompt.
func (s *Store) Remember(message agent.Message) error {
	message = message.Clone()
	if messageEmpty(message) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) > 0 && s.history[len(s.history)-1].Equal(message) {
		return nil
	}
	next := append(cloneMessages(s.history), message)
	next = tailMessages(next, s.historyLimit)
	if err := s.save("history.json", next); err != nil {
		return err
	}
	s.history = next
	return nil
}

// Draft loads a session-specific prompt without consuming it.
func (s *Store) Draft(sessionID string) (agent.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if draft, ok := s.drafts[sessionID]; ok {
		return draft.Clone(), !messageEmpty(draft), nil
	}
	if s.directory == "" {
		return agent.Message{}, false, nil
	}
	var draft agent.Message
	err := s.load(s.draftName(sessionID), &draft)
	if errors.Is(err, os.ErrNotExist) {
		return agent.Message{}, false, nil
	}
	if err != nil {
		return agent.Message{}, false, err
	}
	s.drafts[sessionID] = draft.Clone()
	return draft.Clone(), !messageEmpty(draft), nil
}

// SaveDraft atomically replaces a session draft, or removes it when empty.
func (s *Store) SaveDraft(sessionID string, message agent.Message) error {
	message = message.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	name := s.draftName(sessionID)
	if messageEmpty(message) {
		if s.directory == "" {
			delete(s.drafts, sessionID)
			return nil
		}
		err := os.Remove(s.path(name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		delete(s.drafts, sessionID)
		return nil
	}
	if current, ok := s.drafts[sessionID]; ok && current.Equal(message) {
		return nil
	}
	if err := s.save(name, message); err != nil {
		return err
	}
	s.drafts[sessionID] = message
	return nil
}

// DiscardDraft retires authoring state for a session that no longer exists.
// It is intentionally distinct from saving an empty draft at call sites: the
// caller is expressing a lifecycle transition, not an editor value change.
func (s *Store) DiscardDraft(sessionID string) error {
	return s.SaveDraft(sessionID, agent.Message{})
}

// StashPrompt preserves a prompt independently of its session draft.
func (s *Store) StashPrompt(message agent.Message) (Stash, error) {
	message = message.Clone()
	if messageEmpty(message) {
		return Stash{}, errors.New("cannot stash an empty prompt")
	}
	identity := make([]byte, 8)
	if _, err := io.ReadFull(s.random, identity); err != nil {
		return Stash{}, fmt.Errorf("create stash id: %w", err)
	}
	stash := Stash{ID: hex.EncodeToString(identity), CreatedAt: s.now().UTC(), Message: message}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append(slices.Clone(s.stashes), stash)
	next = tailStashes(next, s.stashLimit)
	if err := s.save("stashes.json", next); err != nil {
		return Stash{}, err
	}
	s.stashes = next
	return cloneStash(stash), nil
}

// Stashes returns newest prompts first.
func (s *Store) Stashes() []Stash {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stash, len(s.stashes))
	for i, stash := range slices.Backward(s.stashes) {
		out[len(s.stashes)-1-i] = cloneStash(stash)
	}
	return out
}

// Stash returns one detached prompt by identity.
func (s *Store) Stash(id string) (Stash, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stash := range s.stashes {
		if stash.ID == id {
			return cloneStash(stash), true
		}
	}
	return Stash{}, false
}

// DeleteStash permanently removes one stash.
func (s *Store) DeleteStash(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.stashes), func(stash Stash) bool { return stash.ID == id })
	if len(next) == len(s.stashes) {
		return false, nil
	}
	if err := s.save("stashes.json", next); err != nil {
		return false, err
	}
	s.stashes = next
	return true, nil
}

// RememberWorkspace moves a workspace to the front of the recent list.
func (s *Store) RememberWorkspace(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return errors.New("workspace path must be absolute")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.workspaces), func(item Workspace) bool { return item.Path == path })
	next = slices.Insert(next, 0, Workspace{Path: path, LastOpened: s.now().UTC()})
	next = next[:min(len(next), s.workspaceLimit)]
	if err := s.save("workspaces.json", next); err != nil {
		return err
	}
	s.workspaces = next
	return nil
}

// Workspaces returns recent workspaces in newest-first order.
func (s *Store) Workspaces() []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.workspaces)
}

type envelope[T any] struct {
	Version int `json:"version"`
	Value   T   `json:"value"`
}

func (s *Store) load(name string, value any) error {
	if s.directory == "" {
		return os.ErrNotExist
	}
	file, err := os.Open(s.path(name))
	if err != nil {
		return err
	}
	defer file.Close()
	var raw envelope[json.RawMessage]
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.Version != formatVersion {
		return fmt.Errorf("unsupported workbench format %d", raw.Version)
	}
	if err := json.Unmarshal(raw.Value, value); err != nil {
		return err
	}
	return nil
}

func (s *Store) loadOptional(name string, value any) error {
	err := s.load(name, value)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) save(name string, value any) error {
	if s.directory == "" {
		return nil
	}
	path := s.path(name)
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".lyra-state-*")
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope[any]{Version: formatVersion, Value: value}); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode state snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync state snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state snapshot: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s *Store) path(name string) string { return pathologize.Join(s.directory, name) }

func (s *Store) draftName(sessionID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sessionID)))
	return filepath.Join("drafts", hex.EncodeToString(digest[:16])+".json")
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func tailMessages(messages []agent.Message, limit int) []agent.Message {
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return cloneMessages(messages)
}

func tailStashes(stashes []Stash, limit int) []Stash {
	if len(stashes) > limit {
		stashes = stashes[len(stashes)-limit:]
	}
	out := make([]Stash, len(stashes))
	for i, stash := range stashes {
		out[i] = cloneStash(stash)
	}
	return out
}

func cloneMessages(messages []agent.Message) []agent.Message {
	out := make([]agent.Message, len(messages))
	for i, message := range messages {
		out[i] = message.Clone()
	}
	return out
}

func cloneStash(stash Stash) Stash {
	stash.Message = stash.Message.Clone()
	return stash
}

func messageEmpty(message agent.Message) bool {
	return strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0
}
