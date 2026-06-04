package storage

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/lyra/internal/service/provider"
)

// FileProviderService persists the [provider.Service] registry to one JSON
// file under the storage home — the provider analog of [FileSessionService].
// The set is tiny (a handful of providers) so the whole file is rewritten
// atomically on every Configure.
//
// The file holds API keys in plaintext, same as config.yaml; it lives under
// $HOME/.lyra (not the repo) and is the user's local state.
type FileProviderService struct {
	mu        sync.RWMutex
	providers map[string]provider.Provider
	path      string
}

var _ provider.Service = (*FileProviderService)(nil)

// NewFileProviderService opens (or creates) the providers file under the
// storage home directory. A missing file starts an empty registry.
func NewFileProviderService() (*FileProviderService, error) {
	dir, err := Home()
	if err != nil {
		return nil, err
	}
	svc := &FileProviderService{
		providers: map[string]provider.Provider{},
		path:      filepath.Join(dir, "providers.json"),
	}
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *FileProviderService) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // first run — empty registry
		}
		return fmt.Errorf("provider storage: read %s: %w", s.path, err)
	}
	var list []provider.Provider
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("provider storage: parse %s: %w", s.path, err)
	}
	for _, p := range list {
		s.providers[p.ID] = p
	}
	return nil
}

// persist rewrites the whole file atomically (write temp + rename). Caller
// holds the write lock.
func (s *FileProviderService) persist() error {
	list := make([]provider.Provider, 0, len(s.providers))
	for _, p := range s.providers {
		list = append(list, p)
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("provider storage: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("provider storage: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("provider storage: rename %s: %w", s.path, err)
	}
	return nil
}

func (s *FileProviderService) List(_ context.Context) ([]provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]provider.Provider, 0, len(s.providers))
	for _, p := range s.providers {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b provider.Provider) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

func (s *FileProviderService) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[id]
	return p, ok, nil
}

func (s *FileProviderService) Configure(_ context.Context, p provider.Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.ID] = p
	return s.persist()
}
