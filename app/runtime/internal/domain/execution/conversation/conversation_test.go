package conversation

import (
	"errors"
	"testing"

	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestMessagesOwnHistoryTransitions(t *testing.T) {
	store := history.NewInMemoryStore()
	messages := NewMessages(store)
	ctx := t.Context()

	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	if err := messages.Seed(ctx, "ses_1", seed); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if count, err := messages.Count(ctx, "ses_1"); err != nil || count != 3 {
		t.Fatalf("Count = %d, %v; want 3, nil", count, err)
	}

	if err := messages.Truncate(ctx, "ses_1", 2); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if err := messages.InjectUser(ctx, "ses_1", "steer"); err != nil {
		t.Fatalf("InjectUser: %v", err)
	}

	got, err := messages.Read(ctx, "ses_1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 || got[0].Text() != "one" || got[1].Text() != "two" || got[2].Text() != "steer" {
		t.Fatalf("history = %#v, want one/two/steer", got)
	}

	if err := messages.Truncate(ctx, "ses_1", -1); err != nil {
		t.Fatalf("clear through Truncate: %v", err)
	}
	if got, err := messages.Read(ctx, "ses_1"); err != nil || len(got) != 0 {
		t.Fatalf("cleared history = %#v, %v; want empty, nil", got, err)
	}
}

func TestMessagesRejectMalformedCommands(t *testing.T) {
	messages := NewMessages(history.NewInMemoryStore())
	ctx := t.Context()

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "read without session", run: func() error { _, err := messages.Read(ctx, ""); return err }, want: errSessionIDRequired},
		{name: "seed without session", run: func() error { return messages.Seed(ctx, "", nil) }, want: errSessionIDRequired},
		{name: "count without session", run: func() error { _, err := messages.Count(ctx, ""); return err }, want: errSessionIDRequired},
		{name: "truncate without session", run: func() error { return messages.Truncate(ctx, "", 0) }, want: errSessionIDRequired},
		{name: "inject without session", run: func() error { return messages.InjectUser(ctx, "", "text") }, want: errSessionIDRequired},
		{name: "inject empty text", run: func() error { return messages.InjectUser(ctx, "ses_1", "") }, want: errTextRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
