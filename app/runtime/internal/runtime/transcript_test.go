package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type transcriptRuntimeStore struct {
	listSessionID string
	runsSessionID string
	items         []transcript.Item
	runs          []transcript.Run
}

var _ transcriptStore = (*transcriptRuntimeStore)(nil)

func (s *transcriptRuntimeStore) List(_ context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	s.listSessionID = sessionID
	return s.items, s.runs, nil
}

func (s *transcriptRuntimeStore) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	s.runsSessionID = sessionID
	return s.runs, nil
}

func (*transcriptRuntimeStore) AppendItem(context.Context, transcript.Item) error { return nil }

func (*transcriptRuntimeStore) PutRun(context.Context, transcript.Run) error { return nil }

func TestRuntimeListTranscript(t *testing.T) {
	store := &transcriptRuntimeStore{
		items: []transcript.Item{{ItemID: "item_1"}},
		runs:  []transcript.Run{{RunID: "run_1"}},
	}
	rt := &Runtime{transcript: store}

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
	store := &transcriptRuntimeStore{runs: []transcript.Run{{RunID: "run_1"}}}
	rt := &Runtime{transcript: store}

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
