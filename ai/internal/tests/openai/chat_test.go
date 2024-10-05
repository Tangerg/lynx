package openai

import (
	"context"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/client"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/models/openai/chat"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

func Test1(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).Build()
	content, err := cli.PromptText("hello! who are you?").
		Call().Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}

func Test2(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	safaguard := advisor.NewSafeGuardAroundAdvisor[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata]([]string{"fuck"})
	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultPrueAdvisors(safaguard).Build()
	content, err := cli.PromptText("sad fuck sdsad").
		Call().Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}

func Test3(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	chatMemory := memory.NewInMemoryChatMemory()
	mem := advisor.NewMessageChatMemory[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata](chatMemory)
	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultAdvisorsWihtParams(map[string]any{
			advisor.ChatMemoryConversationIdKey: "chat_id_o1",
			advisor.ChatMemoryRetrieveSizeKey:   100,
		}, mem).
		Build()
	content, err := cli.PromptText("hello! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	content, err = cli.PromptText("hello! My name is Bob!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! Please name all the names that have appeared before!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}

func Test4(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	chatMemory := memory.NewInMemoryChatMemory()
	mem := advisor.NewMessageChatMemory[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata](chatMemory)
	render := advisor.NewRenderAdvisor[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata]()
	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultAdvisorsWihtParams(map[string]any{
			advisor.ChatMemoryConversationIdKey: "chat_id_o1",
			advisor.ChatMemoryRetrieveSizeKey:   100,
		}, mem, render).
		Build()
	content, err := cli.PromptText("hello! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	content, err = cli.PromptText("hello! My name is Bob!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hello! Please name all the names that have appeared before!").
		Call().
		ResponseValueSlice(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	value := entity.Value()
	for _, val := range value {
		t.Log(val)
	}
	t.Log(entity.Response().Result().Output().Content())
}

func Test5(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	chatMemory := memory.NewInMemoryChatMemory()
	mem := advisor.NewMessageChatMemory[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata](chatMemory)
	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultAdvisorsWihtParams(map[string]any{
			advisor.ChatMemoryConversationIdKey: "chat_id_o1",
			advisor.ChatMemoryRetrieveSizeKey:   100,
		}, mem).
		Build()
	content, err := cli.PromptText("hello! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! My favorite music genre is R&B and Country Music").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hello! Please tell me my name and email address and favorite music genre! If I haven't told you, please leave it blank!").
		Call().
		ResponseValueMap(context.Background(), map[string]any{
			"name":                 "the name of user",
			"email":                "the email of user,",
			"favorite_music_genre": []string{"user favorite music genre eg: Rock", "etc..."},
		})
	if err != nil {
		t.Log(err)
		return
	}
	value := entity.Value()
	for key, val := range value {
		t.Log(key, val)
	}
	t.Log(entity.Response().Result().Output().Content())
	entity, err = cli.PromptText("hello! My email is tom@gmail.com!").
		Call().
		ResponseValueMap(context.Background(), map[string]any{
			"name":                 "the name of user",
			"email":                "the email of user,",
			"favorite_music_genre": []string{"user favorite music genre eg: Rock", "etc..."},
		})
	if err != nil {
		t.Log(err)
		return
	}
	value = entity.Value()
	for key, val := range value {
		t.Log(key, val)
	}
	t.Log(entity.Response().Result().Output().Content())
}

func Test6(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	opts, err := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		Build()
	if err != nil {
		t.Log(err)
		return
	}
	chatMemory := memory.NewInMemoryChatMemory()
	mem := advisor.NewMessageChatMemory[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata](chatMemory)
	render := advisor.NewRenderAdvisor[*chat.OpenAIChatOptions, *metadata.OpenAIChatGenerationMetadata]()

	cli := client.NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultAdvisorsWihtParams(map[string]any{
			advisor.ChatMemoryConversationIdKey: "chat_id_o1",
			advisor.ChatMemoryRetrieveSizeKey:   100,
		}, mem, render).
		Build()
	content, err := cli.PromptText("hello! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hello! My favorite music genre is R&B and Country Music").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hello! Please tell me my name and email address and favorite music genre! If I haven't told you, please leave it blank!").
		Call().
		ResponseValueMap(context.Background(), map[string]any{
			"name":                 "the name of user",
			"email":                "the email of user,",
			"favorite_music_genre": []string{"user favorite music genre eg: Rock", "etc..."},
		})
	if err != nil {
		t.Log(err)
		return
	}
	value := entity.Value()
	for key, val := range value {
		t.Log(key, val)
	}
	t.Log(entity.Response().Result().Output().Content())
}
