// Package arch_test locks provider-facing protocol and constructor boundaries.
package providerconformance_test

import (
	"testing"

	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/anthropic"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/deepseek"
	"github.com/Tangerg/scope/models/fireworks"
	"github.com/Tangerg/scope/models/groq"
	"github.com/Tangerg/scope/models/huggingface"
	"github.com/Tangerg/scope/models/minimax"
	"github.com/Tangerg/scope/models/mistral"
	"github.com/Tangerg/scope/models/moonshot"
	"github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/openrouter"
	"github.com/Tangerg/scope/models/perplexity"
	"github.com/Tangerg/scope/models/together"
	"github.com/Tangerg/scope/models/xai"
	"github.com/Tangerg/scope/models/xiaomi"
	"github.com/Tangerg/scope/models/zhipu"
)

type configValidator interface {
	Validate() error
}

func assertConstructor[Config, Model any](func(Config) (*Model, error)) {}

func TestTargetChatProviderConstructorsCompile(t *testing.T) {
	t.Parallel()

	assertConstructor[alibaba.OpenAIChatConfig, alibaba.OpenAIChat](alibaba.NewOpenAIChat)
	assertConstructor[anthropic.ChatConfig, anthropic.Chat](anthropic.NewChat)
	assertConstructor[anthropic.OpenAIChatConfig, anthropic.OpenAIChat](anthropic.NewOpenAIChat)
	assertConstructor[azureopenai.ChatConfig, azureopenai.Chat](azureopenai.NewChat)
	assertConstructor[deepseek.OpenAIChatConfig, deepseek.OpenAIChat](deepseek.NewOpenAIChat)
	assertConstructor[fireworks.OpenAIChatConfig, fireworks.OpenAIChat](fireworks.NewOpenAIChat)
	assertConstructor[groq.OpenAIChatConfig, groq.OpenAIChat](groq.NewOpenAIChat)
	assertConstructor[huggingface.OpenAIChatConfig, huggingface.OpenAIChat](huggingface.NewOpenAIChat)
	assertConstructor[minimax.OpenAIChatConfig, minimax.OpenAIChat](minimax.NewOpenAIChat)
	assertConstructor[minimax.AnthropicChatConfig, minimax.AnthropicChat](minimax.NewAnthropicChat)
	assertConstructor[mistral.ChatConfig, mistral.Chat](mistral.NewChat)
	assertConstructor[moonshot.OpenAIChatConfig, moonshot.OpenAIChat](moonshot.NewOpenAIChat)
	assertConstructor[moonshot.AnthropicChatConfig, moonshot.AnthropicChat](moonshot.NewAnthropicChat)
	assertConstructor[openai.ChatConfig, openai.Chat](openai.NewChat)
	assertConstructor[openai.ChatConfig, openai.ResponsesChat](openai.NewResponsesChat)
	assertConstructor[openrouter.OpenAIChatConfig, openrouter.OpenAIChat](openrouter.NewOpenAIChat)
	assertConstructor[openrouter.AnthropicChatConfig, openrouter.AnthropicChat](openrouter.NewAnthropicChat)
	assertConstructor[perplexity.OpenAIChatConfig, perplexity.OpenAIChat](perplexity.NewOpenAIChat)
	assertConstructor[together.OpenAIChatConfig, together.OpenAIChat](together.NewOpenAIChat)
	assertConstructor[xai.OpenAIChatConfig, xai.OpenAIChat](xai.NewOpenAIChat)
	assertConstructor[xiaomi.OpenAIChatConfig, xiaomi.OpenAIChat](xiaomi.NewOpenAIChat)
	assertConstructor[xiaomi.AnthropicChatConfig, xiaomi.AnthropicChat](xiaomi.NewAnthropicChat)
	assertConstructor[zhipu.OpenAIChatConfig, zhipu.OpenAIChat](zhipu.NewOpenAIChat)
	assertConstructor[zhipu.AnthropicChatConfig, zhipu.AnthropicChat](zhipu.NewAnthropicChat)
}

func TestTargetChatProviderConfigsValidate(t *testing.T) {
	t.Parallel()

	var (
		_ configValidator = alibaba.OpenAIChatConfig{}
		_ configValidator = anthropic.ChatConfig{}
		_ configValidator = anthropic.OpenAIChatConfig{}
		_ configValidator = azureopenai.ChatConfig{}
		_ configValidator = deepseek.OpenAIChatConfig{}
		_ configValidator = fireworks.OpenAIChatConfig{}
		_ configValidator = groq.OpenAIChatConfig{}
		_ configValidator = huggingface.OpenAIChatConfig{}
		_ configValidator = minimax.OpenAIChatConfig{}
		_ configValidator = minimax.AnthropicChatConfig{}
		_ configValidator = mistral.ChatConfig{}
		_ configValidator = moonshot.OpenAIChatConfig{}
		_ configValidator = moonshot.AnthropicChatConfig{}
		_ configValidator = openai.ChatConfig{}
		_ configValidator = openrouter.OpenAIChatConfig{}
		_ configValidator = openrouter.AnthropicChatConfig{}
		_ configValidator = perplexity.OpenAIChatConfig{}
		_ configValidator = together.OpenAIChatConfig{}
		_ configValidator = xai.OpenAIChatConfig{}
		_ configValidator = xiaomi.OpenAIChatConfig{}
		_ configValidator = xiaomi.AnthropicChatConfig{}
		_ configValidator = zhipu.OpenAIChatConfig{}
		_ configValidator = zhipu.AnthropicChatConfig{}
	)
}
