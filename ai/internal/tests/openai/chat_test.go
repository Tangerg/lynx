package openai

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/client"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/chatmemory"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/modelinvoke"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/outputguide"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/safeguard"
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware/templaterender"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/models/openai/chat"
	"github.com/Tangerg/lynx/ai/models/openai/metadata"
)

type O = *chat.OpenAIChatOptions
type M = *metadata.OpenAIChatGenerationMetadata

func newClient() client.ChatClient[O, M] {
	token := os.Getenv("OPENAI_TOKEN")
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	sg := safeguard.New[O, M]("fuck")
	chatMemory := memory.NewInMemoryChatMemory()
	cm := chatmemory.New[O, M](&chatmemory.Config{
		Store: chatMemory,
	})
	og := outputguide.New[O, M]()
	tr := templaterender.New[O, M]()
	mi := modelinvoke.New[O, M]()
	opts, _ := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4oMini).
		WithStreamChunkFunc(func(ctx context.Context, chunk string) error {
			fmt.Println(chunk)
			return nil
		}).
		Build()
	cli := client.
		NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatOptions(opts).
		DefaultMiddlewaresWithParams(
			map[string]any{},
			sg, cm, og, tr, mi,
		).
		Build()
	return cli
}

func Test1(t *testing.T) {
	cli := newClient()
	content, err := cli.PromptText("hello! who are you?").
		Stream().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}

func Test2(t *testing.T) {
	cli := newClient()
	content, err := cli.PromptText("sad fuck sdsad").
		Call().Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}

func Test3(t *testing.T) {
	cli := newClient()
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
	cli := newClient()
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
	cli := newClient()
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
	cli := newClient()
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
