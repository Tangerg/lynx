package arch_test

import (
	"testing"

	"github.com/Tangerg/lynx/models/ollama"
)

func TestChatConstructorsCompile(t *testing.T) {
	t.Parallel()
	var (
		_ func(ollama.ChatConfig) (*ollama.Chat, error)             = ollama.NewChat
		_ func(ollama.OpenAIChatConfig) (*ollama.OpenAIChat, error) = ollama.NewOpenAIChat
	)
}
