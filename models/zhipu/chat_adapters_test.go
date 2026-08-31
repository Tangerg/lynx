package zhipu

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestChatConfigsValidateCredentialAndOptions(t *testing.T) {
	t.Parallel()

	invalidOptions := chat.Options{Stop: []string{""}}
	tests := []struct {
		name string
		err  error
	}{
		{name: "credential", err: (ChatConfig{}).Validate()},
		{name: "options", err: (ChatConfig{APIKey: "key", DefaultOptions: invalidOptions}).Validate()},
		{name: "messages options", err: (MessagesConfig{APIKey: "key", DefaultOptions: invalidOptions}).Validate()},
	}
	for _, test := range tests {
		if test.err == nil {
			t.Fatalf("%s validation error = nil", test.name)
		}
	}
}

func TestChatConstructorsProduceProtocolAdapters(t *testing.T) {
	t.Parallel()

	model, err := NewChat(ChatConfig{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if model == nil {
		t.Fatal("NewChat() = nil")
	}

	_, invalidErr := NewChat(ChatConfig{})
	if invalidErr == nil {
		t.Fatal("NewChat(invalid config) error = nil")
	}

	anthropicModel, err := NewMessages(MessagesConfig{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if anthropicModel == nil {
		t.Fatal("NewMessages() = nil")
	}
	_, invalidErr = NewMessages(MessagesConfig{})
	if invalidErr == nil {
		t.Fatal("NewMessages(invalid config) error = nil")
	}
}
