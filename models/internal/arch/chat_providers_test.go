// Package arch_test locks provider-facing protocol and constructor boundaries.
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
		_ func(alibaba.OpenAIChatConfig) (*alibaba.OpenAIChat, error)             = alibaba.NewOpenAIChat
		_ func(anthropic.ChatConfig) (*anthropic.Chat, error)                     = anthropic.NewChat
		_ func(anthropic.OpenAIChatConfig) (*anthropic.OpenAIChat, error)         = anthropic.NewOpenAIChat
		_ func(azureopenai.ChatConfig) (*azureopenai.Chat, error)                 = azureopenai.NewChat
		_ func(context.Context, bedrock.ChatConfig) (*bedrock.Chat, error)        = bedrock.NewChat
		_ func(deepseek.OpenAIChatConfig) (*deepseek.OpenAIChat, error)           = deepseek.NewOpenAIChat
		_ func(fireworks.OpenAIChatConfig) (*fireworks.OpenAIChat, error)         = fireworks.NewOpenAIChat
		_ func(google.ChatConfig) (*google.Chat, error)                           = google.NewChat
		_ func(google.OpenAIChatConfig) (*google.OpenAIChat, error)               = google.NewOpenAIChat
		_ func(groq.OpenAIChatConfig) (*groq.OpenAIChat, error)                   = groq.NewOpenAIChat
		_ func(huggingface.OpenAIChatConfig) (*huggingface.OpenAIChat, error)     = huggingface.NewOpenAIChat
		_ func(minimax.OpenAIChatConfig) (*minimax.OpenAIChat, error)             = minimax.NewOpenAIChat
		_ func(minimax.AnthropicChatConfig) (*minimax.AnthropicChat, error)       = minimax.NewAnthropicChat
		_ func(mistral.ChatConfig) (*mistral.Chat, error)                         = mistral.NewChat
		_ func(moonshot.OpenAIChatConfig) (*moonshot.OpenAIChat, error)           = moonshot.NewOpenAIChat
		_ func(moonshot.AnthropicChatConfig) (*moonshot.AnthropicChat, error)     = moonshot.NewAnthropicChat
		_ func(ollama.ChatConfig) (*ollama.Chat, error)                           = ollama.NewChat
		_ func(ollama.OpenAIChatConfig) (*ollama.OpenAIChat, error)               = ollama.NewOpenAIChat
		_ func(openai.ChatConfig) (*openai.Chat, error)                           = openai.NewChat
		_ func(openai.ChatConfig) (*openai.ResponsesChat, error)                  = openai.NewResponsesChat
		_ func(openrouter.OpenAIChatConfig) (*openrouter.OpenAIChat, error)       = openrouter.NewOpenAIChat
		_ func(openrouter.AnthropicChatConfig) (*openrouter.AnthropicChat, error) = openrouter.NewAnthropicChat
		_ func(perplexity.OpenAIChatConfig) (*perplexity.OpenAIChat, error)       = perplexity.NewOpenAIChat
		_ func(together.OpenAIChatConfig) (*together.OpenAIChat, error)           = together.NewOpenAIChat
		_ func(vertexai.ChatConfig) (*vertexai.Chat, error)                       = vertexai.NewChat
		_ func(xai.OpenAIChatConfig) (*xai.OpenAIChat, error)                     = xai.NewOpenAIChat
		_ func(xiaomi.OpenAIChatConfig) (*xiaomi.OpenAIChat, error)               = xiaomi.NewOpenAIChat
		_ func(xiaomi.AnthropicChatConfig) (*xiaomi.AnthropicChat, error)         = xiaomi.NewAnthropicChat
		_ func(zhipu.OpenAIChatConfig) (*zhipu.OpenAIChat, error)                 = zhipu.NewOpenAIChat
		_ func(zhipu.AnthropicChatConfig) (*zhipu.AnthropicChat, error)           = zhipu.NewAnthropicChat
	)
}
