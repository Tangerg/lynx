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

	if err := rt.SeedHistory(context.Background(), "ses_2", []chat.Message{chat.NewAssistantMessage("done")}); err != nil {
		t.Fatalf("SeedHistory err = %v", err)
	}
	if store.seedSession != "ses_2" || len(store.seedMessages) != 1 {
		t.Fatalf("seed session=%q msgs=%+v", store.seedSession, store.seedMessages)
	}

	count, err := rt.MessageCount(context.Background(), "ses_3")
	if err != nil {
		t.Fatalf("MessageCount err = %v", err)
	}
	if store.countSession != "ses_3" || count != 3 {
		t.Fatalf("count session=%q count=%d", store.countSession, count)
	}

	if err := rt.TruncateMessages(context.Background(), "ses_4", 2); err != nil {
		t.Fatalf("TruncateMessages err = %v", err)
	}
	if store.truncateSession != "ses_4" || store.keepN != 2 {
		t.Fatalf("truncate session=%q keepN=%d", store.truncateSession, store.keepN)
	}
}

type fakeHistoryStore struct {
	messages []chat.Message
	count    int

	readSession     string
	seedSession     string
	seedMessages    []chat.Message
	countSession    string
	truncateSession string
	keepN           int
}

func (s *fakeHistoryStore) Read(_ context.Context, sessionID string) ([]chat.Message, error) {
	s.readSession = sessionID
	return s.messages, nil
}

func (s *fakeHistoryStore) Seed(_ context.Context, sessionID string, msgs []chat.Message) error {
	s.seedSession = sessionID
	s.seedMessages = msgs
	return nil
}

func (s *fakeHistoryStore) Count(_ context.Context, sessionID string) (int, error) {
	s.countSession = sessionID
	return s.count, nil
}

func (s *fakeHistoryStore) Truncate(_ context.Context, sessionID string, keepN int) error {
	s.truncateSession = sessionID
	s.keepN = keepN
	return nil
}
