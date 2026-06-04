package session

import "context"

// NewInMemoryService returns a [Service] backed by an in-process
// map (via [Repo]). Suitable for tests; durable backends
// (FileSessionService in lyra/internal/storage) wrap the same
// Repo with a persist hook.
//
// All methods are safe for concurrent use.
func NewInMemoryService() Service {
	return &inMemoryService{repo: NewRepo()}
}

// inMemoryService is a thin adapter that maps [Service]'s
// context+error contract onto the lock-only [Repo] surface.
type inMemoryService struct {
	repo *Repo
}

func (s *inMemoryService) List(_ context.Context) ([]Session, error) {
	return s.repo.List(), nil
}

func (s *inMemoryService) Get(_ context.Context, id string) (Session, error) {
	sess, ok := s.repo.Get(id)
	if !ok {
		return Session{}, ErrNotFound
	}
	return sess, nil
}

func (s *inMemoryService) Create(_ context.Context, title, cwd string) (Session, error) {
	return s.repo.Create(title, cwd), nil
}

// Fork mirrors [Repo.Fork] but adapts the "missing parent" signal
// into the public [ErrNotFound] sentinel.
func (s *inMemoryService) Fork(_ context.Context, parentID, atMessageID string) (Session, error) {
	child, ok := s.repo.Fork(parentID, atMessageID)
	if !ok {
		return Session{}, ErrNotFound
	}
	return child, nil
}

// Delete is idempotent — deleting an unknown id is not an error.
func (s *inMemoryService) Delete(_ context.Context, id string) error {
	s.repo.Delete(id)
	return nil
}

// Touch refreshes UpdatedAt + bumps TurnCount. Lives on
// *inMemoryService rather than [Service] because it's
// implementation-detail bookkeeping, not part of the public
// surface that transport adapters expose.
func (s *inMemoryService) Touch(id string) error {
	if !s.repo.Touch(id) {
		return ErrNotFound
	}
	return nil
}
