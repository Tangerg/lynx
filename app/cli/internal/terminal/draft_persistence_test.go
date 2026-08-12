package terminal

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type recordingDraftRepository struct {
	mu           sync.Mutex
	writes       []draftSnapshot
	active       int
	maxActive    int
	first        chan struct{}
	releaseFirst chan struct{}
}

func (repository *recordingDraftRepository) SaveDraft(sessionID string, message agent.Message) error {
	repository.mu.Lock()
	repository.active++
	repository.maxActive = max(repository.maxActive, repository.active)
	index := len(repository.writes)
	repository.writes = append(repository.writes, draftSnapshot{sessionID: sessionID, message: message.Clone()})
	if index == 0 && repository.first != nil {
		close(repository.first)
	}
	repository.mu.Unlock()

	if index == 0 && repository.releaseFirst != nil {
		<-repository.releaseFirst
	}

	repository.mu.Lock()
	repository.active--
	repository.mu.Unlock()
	return nil
}

func (repository *recordingDraftRepository) snapshot() ([]draftSnapshot, int) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return append([]draftSnapshot(nil), repository.writes...), repository.maxActive
}

func TestDraftPersistenceSerializesAndCoalescesWrites(t *testing.T) {
	repository := &recordingDraftRepository{first: make(chan struct{}), releaseFirst: make(chan struct{})}
	results := make(chan draftPersistenceResult, 3)
	persistence := newDraftPersistence(repository, func(result draftPersistenceResult) { results <- result })
	persistence.Schedule("session", agent.Message{Text: "first"})
	select {
	case <-repository.first:
	case <-time.After(time.Second):
		t.Fatal("first autosave did not start")
	}

	persistence.Schedule("session", agent.Message{Text: "superseded"})
	persistence.Schedule("session", agent.Message{Text: "latest"})
	close(repository.releaseFirst)
	for range 2 {
		select {
		case <-results:
		case <-time.After(time.Second):
			t.Fatal("autosave result did not arrive")
		}
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, maxActive := repository.snapshot()
	if maxActive != 1 {
		t.Fatalf("maximum concurrent writes = %d, want 1", maxActive)
	}
	if len(writes) != 2 || writes[0].message.Text != "first" || writes[1].message.Text != "latest" {
		t.Fatalf("writes = %+v, want first then latest", writes)
	}
}

func TestDraftPersistenceFlushSupersedesPendingAutosave(t *testing.T) {
	repository := &recordingDraftRepository{first: make(chan struct{}), releaseFirst: make(chan struct{})}
	persistence := newDraftPersistence(repository, nil)
	persistence.Schedule("session", agent.Message{Text: "pending"})
	select {
	case <-repository.first:
	case <-time.After(time.Second):
		t.Fatal("pending autosave did not start")
	}
	flushed := make(chan error, 1)
	go func() { flushed <- persistence.Flush("session", agent.Message{Text: "barrier"}) }()
	close(repository.releaseFirst)
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, maxActive := repository.snapshot()
	if maxActive != 1 || len(writes) != 2 || writes[0].message.Text != "pending" || writes[1].message.Text != "barrier" {
		t.Fatalf("writes = %+v, max concurrency = %d", writes, maxActive)
	}
}

func TestDraftPersistenceCloseFlushesPendingAutosave(t *testing.T) {
	repository := &recordingDraftRepository{}
	persistence := newDraftPersistence(repository, nil)
	persistence.Schedule("session", agent.Message{Text: "last visible value"})
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, _ := repository.snapshot()
	if len(writes) != 1 || writes[0].message.Text != "last visible value" {
		t.Fatalf("writes = %+v", writes)
	}
	if err := persistence.Flush("session", agent.Message{Text: "too late"}); !errors.Is(err, errDraftPersistenceClosed) {
		t.Fatalf("flush after close error = %v", err)
	}
}
