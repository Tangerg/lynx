package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestRuntimeHistoryPorts(t *testing.T) {
	store := &fakeHistoryStore{
		messages: []chat.Message{chat.NewUserMessage("hello")},
		count:    3,
	}
	rt := &Runtime{history: store}

	msgs, err := rt.ReadHistory(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("ReadHistory err = %v", err)
	}
	if store.readSession != "ses_1" || len(msgs) != 1 {
		t.Fatalf("read session=%q msgs=%+v", store.readSession, msgs)
	}

	count, err := rt.MessageCount(context.Background(), "ses_3")
	if err != nil {
		t.Fatalf("MessageCount err = %v", err)
	}
	if store.countSession != "ses_3" || count != 3 {
		t.Fatalf("count session=%q count=%d", store.countSession, count)
	}
}

type fakeHistoryStore struct {
	messages []chat.Message
	count    int

	readSession  string
	countSession string
}

func (s *fakeHistoryStore) Read(_ context.Context, sessionID string) ([]chat.Message, error) {
	s.readSession = sessionID
	return s.messages, nil
}

func (s *fakeHistoryStore) Count(_ context.Context, sessionID string) (int, error) {
	s.countSession = sessionID
	return s.count, nil
}
