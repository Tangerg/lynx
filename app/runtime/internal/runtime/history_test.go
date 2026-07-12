package runtime

import (
	"context"
	"testing"
)

func TestRuntimeMessageCount(t *testing.T) {
	store := &fakeHistoryStore{count: 3}
	rt := &Runtime{history: store}

	count, err := rt.MessageCount(context.Background(), "ses_3")
	if err != nil {
		t.Fatalf("MessageCount err = %v", err)
	}
	if store.countSession != "ses_3" || count != 3 {
		t.Fatalf("count session=%q count=%d", store.countSession, count)
	}
}

type fakeHistoryStore struct {
	count        int
	countSession string
}

func (s *fakeHistoryStore) Count(_ context.Context, sessionID string) (int, error) {
	s.countSession = sessionID
	return s.count, nil
}
