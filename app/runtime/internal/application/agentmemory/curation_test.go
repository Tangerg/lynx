package agentmemory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
)

type fakeCurationStore struct {
	published bool
	err       error
}

type singleWinnerCurationStore struct {
	fakeCurationStore
	won atomic.Bool
}

func (s *singleWinnerCurationStore) Reconcile(context.Context, string, int64, int64, []string, time.Time) (bool, error) {
	return s.won.CompareAndSwap(false, true), nil
}

func (f *fakeCurationStore) AppendLedger(context.Context, domain.FactBatch) ([]domain.LedgerFact, error) {
	return nil, f.err
}

func (f *fakeCurationStore) PendingLedger(context.Context, string, int64, int) ([]domain.LedgerFact, error) {
	return nil, f.err
}

func (f *fakeCurationStore) State(context.Context, string) (domain.State, error) {
	return domain.State{}, f.err
}

func (f *fakeCurationStore) Reconcile(context.Context, string, int64, int64, []string, time.Time) (bool, error) {
	return f.published, f.err
}

func (f *fakeCurationStore) Items(context.Context, domain.Scope, string) ([]domain.Item, error) {
	return nil, f.err
}

func TestCurationReconcilePublishesOnlyNewGeneration(t *testing.T) {
	store := &fakeCurationStore{published: true}
	var notices []invalidation.Notice
	c := NewCuration(CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		notices = append(notices, notice)
	}})

	published, err := c.PublishGeneration(t.Context(), "/repo", 0, 4, []string{"fact"}, time.Now())
	if err != nil || !published {
		t.Fatalf("Reconcile = (%t, %v), want (true, nil)", published, err)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.AgentMemory {
		t.Fatalf("notices = %+v, want one AgentMemory invalidation", notices)
	}

	store.published = false
	published, err = c.PublishGeneration(t.Context(), "/repo", 0, 4, []string{"stale"}, time.Now())
	if err != nil || published {
		t.Fatalf("stale Reconcile = (%t, %v), want (false, nil)", published, err)
	}
	if len(notices) != 1 {
		t.Fatalf("lost CAS notices = %+v", notices)
	}
}

func TestCurationFailureAndLedgerWritesDoNotPublish(t *testing.T) {
	wantErr := errors.New("store unavailable")
	store := &fakeCurationStore{published: true, err: wantErr}
	var notices []invalidation.Notice
	c := NewCuration(CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		notices = append(notices, notice)
	}})

	if published, err := c.PublishGeneration(t.Context(), "/repo", 0, 4, nil, time.Now()); published || !errors.Is(err, wantErr) {
		t.Fatalf("failed Reconcile = (%t, %v), want (false, %v)", published, err, wantErr)
	}
	store.err = nil
	if _, err := c.AppendLedger(t.Context(), domain.FactBatch{}); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatalf("non-public writes published notices = %+v", notices)
	}
}

func TestConcurrentCurationPublishesExactlyOneWinningGeneration(t *testing.T) {
	store := &singleWinnerCurationStore{}
	var notices atomic.Int32
	c := NewCuration(CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		if notice.Resource != invalidation.AgentMemory {
			t.Errorf("notice = %+v, want AgentMemory", notice)
		}
		notices.Add(1)
	}})

	const contenders = 32
	var published atomic.Int32
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		go func() {
			defer group.Done()
			won, err := c.PublishGeneration(t.Context(), "/repo", 0, 4, []string{"fact"}, time.Now())
			if err != nil {
				t.Errorf("Reconcile error = %v", err)
			}
			if won {
				published.Add(1)
			}
		}()
	}
	group.Wait()
	if got := published.Load(); got != 1 {
		t.Fatalf("published generations = %d, want 1", got)
	}
	if got := notices.Load(); got != 1 {
		t.Fatalf("AgentMemory invalidations = %d, want 1", got)
	}
}

func TestCurationUnavailableFailsExplicitly(t *testing.T) {
	c := NewCuration(CurationConfig{})
	if _, err := c.AppendLedger(t.Context(), domain.FactBatch{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("AppendLedger error = %v, want ErrUnavailable", err)
	}
	if _, err := c.PublishGeneration(t.Context(), "/repo", 0, 1, nil, time.Now()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("PublishGeneration error = %v, want ErrUnavailable", err)
	}
}
