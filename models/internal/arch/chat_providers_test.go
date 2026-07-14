// Package arch_test locks the complete provider-facing Core chat constructor
// surface while the legacy model API is being removed.
package arch_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/bedrock"
	"github.com/Tangerg/lynx/models/deepseek"
	"github.com/Tangerg/lynx/models/fireworks"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/groq"
	"github.com/Tangerg/lynx/models/huggingface"
	"github.com/Tangerg/lynx/models/minimax"
	"github.com/Tangerg/lynx/models/mistral"
	"github.com/Tangerg/lynx/models/moonshot"
	"github.com/Tangerg/lynx/models/ollama"
	"github.com/Tangerg/lynx/models/openai"
	"github.com/Tangerg/lynx/models/openrouter"
	"github.com/Tangerg/lynx/models/perplexity"
	"github.com/Tangerg/lynx/models/together"
	"github.com/Tangerg/lynx/models/vertexai"
	"github.com/Tangerg/lynx/models/xai"
	"github.com/Tangerg/lynx/models/xiaomi"
	"github.com/Tangerg/lynx/models/zhipu"
)

func TestTargetChatProviderConstructorsCompile(t *testing.T) {
	t.Parallel()

	var (
		_ func(alibaba.OpenAIChatConfig) (*openai.Chat, error)             = alibaba.NewOpenAIChat
		_ func(anthropic.ChatConfig) (*anthropic.Chat, error)              = anthropic.NewChat
		_ func(anthropic.OpenAIChatConfig) (*openai.Chat, error)           = anthropic.NewOpenAIChat
		_ func(azureopenai.ChatConfig) (*openai.Chat, error)               = azureopenai.NewChat
		_ func(context.Context, bedrock.ChatConfig) (*bedrock.Chat, error) = bedrock.NewChat
		_ func(deepseek.OpenAIChatConfig) (*openai.Chat, error)            = deepseek.NewOpenAIChat
		_ func(fireworks.OpenAIChatConfig) (*openai.Chat, error)           = fireworks.NewOpenAIChat
		_ func(google.ChatConfig) (*google.Chat, error)                    = google.NewChat
		_ func(google.OpenAIChatConfig) (*openai.Chat, error)              = google.NewOpenAIChat
		_ func(groq.OpenAIChatConfig) (*openai.Chat, error)                = groq.NewOpenAIChat
		_ func(huggingface.OpenAIChatConfig) (*openai.Chat, error)         = huggingface.NewOpenAIChat
		_ func(minimax.OpenAIChatConfig) (*openai.Chat, error)             = minimax.NewOpenAIChat
		_ func(minimax.AnthropicChatConfig) (*anthropic.Chat, error)       = minimax.NewAnthropicChat
		_ func(mistral.OpenAIChatConfig) (*openai.Chat, error)             = mistral.NewOpenAIChat
		_ func(moonshot.OpenAIChatConfig) (*openai.Chat, error)            = moonshot.NewOpenAIChat
		_ func(moonshot.AnthropicChatConfig) (*anthropic.Chat, error)      = moonshot.NewAnthropicChat
		_ func(ollama.ChatConfig) (*ollama.Chat, error)                    = ollama.NewChat
		_ func(ollama.OpenAIChatConfig) (*openai.Chat, error)              = ollama.NewOpenAIChat
		_ func(openai.ChatConfig) (*openai.Chat, error)                    = openai.NewChat
		_ func(openai.ChatConfig) (*openai.ResponsesChat, error)           = openai.NewResponsesChat
		_ func(openrouter.OpenAIChatConfig) (*openai.Chat, error)          = openrouter.NewOpenAIChat
		_ func(openrouter.AnthropicChatConfig) (*anthropic.Chat, error)    = openrouter.NewAnthropicChat
		_ func(perplexity.OpenAIChatConfig) (*openai.Chat, error)          = perplexity.NewOpenAIChat
		_ func(together.OpenAIChatConfig) (*openai.Chat, error)            = together.NewOpenAIChat
		_ func(vertexai.ChatConfig) (*google.Chat, error)                  = vertexai.NewChat
		_ func(xai.OpenAIChatConfig) (*openai.Chat, error)                 = xai.NewOpenAIChat
		_ func(xiaomi.OpenAIChatConfig) (*openai.Chat, error)              = xiaomi.NewOpenAIChat
		_ func(xiaomi.AnthropicChatConfig) (*anthropic.Chat, error)        = xiaomi.NewAnthropicChat
		_ func(zhipu.OpenAIChatConfig) (*openai.Chat, error)               = zhipu.NewOpenAIChat
		_ func(zhipu.AnthropicChatConfig) (*anthropic.Chat, error)         = zhipu.NewAnthropicChat
	)
}
