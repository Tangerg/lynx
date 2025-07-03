package chat

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/converter"
	"github.com/Tangerg/lynx/ai/model/model"
	"github.com/Tangerg/lynx/ai/providers/models/openai"
	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/openai/openai-go/option"
	"io"
	"os"
	"testing"
)

var (
	baseURL   = "https://api.moonshot.cn/v1"
	baseModel = "moonshot-v1-8k-vision-preview"
)

func newApiKey() model.ApiKey {
	k := os.Getenv("apiKey")
	return model.NewApiKey(k)
}
func newChatModel() *openai.ChatModel {
	defaultOptions := openai.NewChatOptionsBuilder().WithModel(baseModel).MustBuild()
	return assert.ErrorIsNil(openai.NewChatModel(newApiKey(), defaultOptions, option.WithBaseURL(baseURL)))
}

func newChatClient() *Client {
	newChatModel()
	return NewClientBuilder().WithChatModel(newChatModel()).MustBuild()
}

func TestClient_Call_Chat(t *testing.T) {
	client := newChatClient()
	text, err := client.Chat().Call().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
}

func TestClient_Call_ChatText(t *testing.T) {
	client := newChatClient()
	text, err := client.ChatText("Hi!,How are you!").Call().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
}

func TestClient_Call_ChatText_List(t *testing.T) {
	client := newChatClient()
	list, err := client.ChatText("list five color").Call().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("count:", len(list))
	for _, item := range list {
		t.Log(item)
	}
}

func TestClient_Call_ChatText_Map(t *testing.T) {
	client := newChatClient()
	m, err := client.ChatText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information").Call().Map(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("count:", len(m))
	for k, v := range m {
		t.Log(k, v)
	}
}

func TestClient_Call_ChatText_Any(t *testing.T) {
	type user struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"Email"`
	}

	client := newChatClient()
	u, err := client.ChatText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information").Call().Any(context.Background(), converter.JSONAsAnyOf[*user]())
	if err != nil {
		t.Fatal(err)
	}

	user1 := u.(*user)
	t.Log(user1.Name, user1.Age, user1.Email)
}

func TestClient_Call_ChatRequest(t *testing.T) {
	client := newChatClient()
	chatReq, _ := chat.NewRequest([]messages.Message{messages.NewUserMessage("Hi!,How are you!")})
	text, err := client.ChatRequest(chatReq).Call().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
}

func TestClient_Stream_Chat(t *testing.T) {
	client := newChatClient()
	streamer, err := client.Chat().Stream().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for {
		result, err := streamer.Read(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if result.Error() != nil {
			t.Fatal(result.Error())
		}
		t.Log(result.Value())
	}
}

func TestClient_Stream_ChatText(t *testing.T) {
	client := newChatClient()
	streamer, err := client.ChatText("Hi!,How are you!").Stream().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for {
		result, err := streamer.Read(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if result.Error() != nil {
			t.Fatal(result.Error())
		}
		t.Log(result.Value())
	}
}

func TestClient_Stream_ChatRequest(t *testing.T) {
	client := newChatClient()
	chatReq, _ := chat.NewRequest([]messages.Message{messages.NewUserMessage("Hi!,How are you!")})
	streamer, err := client.ChatRequest(chatReq).Stream().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for {
		result, err := streamer.Read(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if result.Error() != nil {
			t.Fatal(result.Error())
		}
		t.Log(result.Value())
	}
}
