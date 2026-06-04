package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/lyra/internal/service/provider"
)

// FileProviderService persists the [provider.Service] registry to one JSON
// file under the storage home — the provider analog of [FileSessionService].
// The in-memory shape (map + lock + CRUD) is owned by [provider.Repo]; this
// type composes it with a persist-on-Configure hook. The set is tiny (a
// handful of providers) so the whole file is rewritten atomically each time.
//
// The file holds API keys in plaintext, same as config.yaml; it lives under
// $HOME/.lyra (not the repo) and is the user's local state.
type FileProviderService struct {
	repo *provider.Repo
	path string
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
		repo: provider.NewRepo(),
		path: filepath.Join(dir, "providers.json"),
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
	s.repo.Restore(list)
	return nil
}

// persist rewrites the whole file atomically (write temp + rename).
func (s *FileProviderService) persist() error {
	raw, err := json.MarshalIndent(s.repo.List(), "", "  ")
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
	return s.repo.List(), nil
}

func (s *FileProviderService) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	p, ok := s.repo.Get(id)
	return p, ok, nil
}

func (s *FileProviderService) Configure(_ context.Context, p provider.Provider) error {
	s.repo.Set(p)
	return s.persist()
}
