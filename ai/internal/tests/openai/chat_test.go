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

var chatMemory = memory.NewInMemoryChatMemory()

type O = *chat.OpenAIChatOptions
type M = *metadata.OpenAIChatGenerationMetadata

func newClient() client.ChatClient[O, M] {
	token := os.Getenv("OPENAI_TOKEN")
	openAIChatModel := chat.NewOpenAIChatModel(chat.OpenAIChatModelConfig{
		Token: token,
	})
	sg := safeguard.New[O, M]("fuck")

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
	content, err := cli.PromptText("hi! who are you?").
		Call().Content(context.Background())
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
	content, err := cli.PromptText("hi! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	content, err = cli.PromptText("hi! My name is Bob!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! Please name all the names that have appeared before!").
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
	content, err := cli.PromptText("hi! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	content, err = cli.PromptText("hi! My name is Bob!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! What is my name?").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hi! Please name all the names that have appeared before!").
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
	content, err := cli.PromptText("hi! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! My favorite music genre is R&B and Country Music").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hi! Please tell me my name and email address and favorite music genre! If I haven't told you, please leave it blank!").
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
	t.Log(entity.Response().Result().Output().Content())
	entity, err = cli.PromptText("hi! My email is tom@gmail.com!").
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
	t.Log(entity.Response().Result().Output().Content())
}

func Test6(t *testing.T) {
	cli := newClient()
	content, err := cli.PromptText("hi! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	content, err = cli.PromptText("hi! My favorite music genre is R&B and Country Music").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
	entity, err := cli.PromptText("hi! Please tell me my name and email address and favorite music genre! If I haven't told you, please leave it blank!").
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

func Test7(t *testing.T) {
	cli := newClient()
	example := map[string]any{
		"phone_number":         "the phone number of user",
		"name":                 "the name of user",
		"email":                "the email of user,",
		"gender":               "the gender of user, only male of female",
		"favorite_music_genre": []string{"user favorite music genre eg: Rock", "etc..."},
	}
	_, err := cli.PromptText("hi! My name is Tom!").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("My favorite music genre is R&B and Country Music").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("Hi! Please provide my basic personal information. If I haven't shared it with you, please leave it blank.").
		Call().
		ResponseValueMap(context.Background(), example)
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("My email address is tom@gmail.com!").
		Call().
		ResponseValueMap(context.Background(), example)
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("My phone number is +8613912341234").
		Call().
		ResponseValueMap(context.Background(), example)
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("You might be able to infer my gender from my name.").
		Call().
		ResponseValueMap(context.Background(), example)
	if err != nil {
		t.Log(err)
		return
	}
	_, err = cli.PromptText("I just underwent gender reassignment surgery.").
		Call().
		ResponseValueMap(context.Background(), example)
	if err != nil {
		t.Log(err)
		return
	}
	msgs, _ := chatMemory.Get(context.Background(), "", 1000)
	for _, msg := range msgs {
		t.Log(msg.Type()+":", msg.Content())
	}
}
