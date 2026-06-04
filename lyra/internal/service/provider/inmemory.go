package provider

import "context"

// inMemory is the default [Service] — a thin context-aware wrapper over
// [Repo]. State is lost on restart; the file / sqlite backends persist it.
type inMemory struct {
	repo *Repo
}

// NewInMemory returns the in-process registry.
func NewInMemory() Service {
	return &inMemory{repo: NewRepo()}
}

var _ Service = (*inMemory)(nil)

func (s *inMemory) List(_ context.Context) ([]Provider, error) {
	return s.repo.List(), nil
}

func (s *inMemory) Get(_ context.Context, id string) (Provider, bool, error) {
	p, ok := s.repo.Get(id)
	return p, ok, nil
}

func (s *inMemory) Configure(_ context.Context, p Provider) error {
	s.repo.Set(p)
	return nil
}
