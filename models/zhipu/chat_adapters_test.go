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
		{name: "OpenAI credential", err: (OpenAIChatConfig{}).Validate()},
		{name: "Anthropic credential", err: (AnthropicChatConfig{}).Validate()},
		{name: "OpenAI options", err: (OpenAIChatConfig{APIKey: "key", DefaultOptions: invalidOptions}).Validate()},
		{name: "Anthropic options", err: (AnthropicChatConfig{APIKey: "key", DefaultOptions: invalidOptions}).Validate()},
	}
	for _, test := range tests {
		if test.err == nil {
			t.Fatalf("%s validation error = nil", test.name)
		}
	}
}

func TestChatConstructorsProduceProtocolAdapters(t *testing.T) {
	t.Parallel()

	openAIChat, err := NewOpenAIChat(OpenAIChatConfig{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if openAIChat == nil {
		t.Fatal("NewOpenAIChat() = nil")
	}

	anthropicChat, err := NewAnthropicChat(AnthropicChatConfig{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if anthropicChat == nil {
		t.Fatal("NewAnthropicChat() = nil")
	}

	if _, err := NewOpenAIChat(OpenAIChatConfig{}); err == nil {
		t.Fatal("NewOpenAIChat(invalid config) error = nil")
	}
	if _, err := NewAnthropicChat(AnthropicChatConfig{}); err == nil {
		t.Fatal("NewAnthropicChat(invalid config) error = nil")
	}
}
