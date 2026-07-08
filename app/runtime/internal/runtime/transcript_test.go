package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type transcriptStore struct {
	listSessionID string
	runsSessionID string
	items         []transcript.Item
	runs          []transcript.Run
}

func (s *transcriptStore) List(_ context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	s.listSessionID = sessionID
	return s.items, s.runs, nil
}

func (s *transcriptStore) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	s.runsSessionID = sessionID
	return s.runs, nil
}

func TestRuntimeListTranscript(t *testing.T) {
	store := &transcriptStore{
		items: []transcript.Item{{ItemID: "item_1"}},
		runs:  []transcript.Run{{RunID: "run_1"}},
	}
	rt := &Runtime{transcriptContent: store}

	items, runs, err := rt.ListTranscript(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("list transcript: %v", err)
	}
	if store.listSessionID != "ses_1" {
		t.Fatalf("store session = %q, want ses_1", store.listSessionID)
	}
	if len(items) != 1 || items[0].ItemID != "item_1" || len(runs) != 1 || runs[0].RunID != "run_1" {
		t.Fatalf("items=%+v runs=%+v", items, runs)
	}
}

func TestRuntimeListTranscriptRuns(t *testing.T) {
	store := &transcriptStore{runs: []transcript.Run{{RunID: "run_1"}}}
	rt := &Runtime{transcriptRuns: store}

	runs, err := rt.ListTranscriptRuns(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("list transcript runs: %v", err)
	}
	if store.runsSessionID != "ses_1" {
		t.Fatalf("store session = %q, want ses_1", store.runsSessionID)
	}
	if len(runs) != 1 || runs[0].RunID != "run_1" {
		t.Fatalf("runs = %+v", runs)
	}
}
