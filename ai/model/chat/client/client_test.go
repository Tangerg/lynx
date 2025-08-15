package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/converter"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/tool"
	"github.com/Tangerg/lynx/ai/providers/models/openai"
	"github.com/Tangerg/lynx/pkg/assert"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
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
	return assert.ErrorIsNil(
		NewClient(
			assert.ErrorIsNil(
				NewConfig(newChatModel()),
			),
		),
	)
}

type weatherRequest struct {
	StartAt  int64  `json:"after" jsonschema_description:"Start time of weather query, second level timestamp in Unix format"`
	EndAt    int64  `json:"before" jsonschema_description:"End time of weather query, second level timestamp in Unix format"`
	Location string `json:"location" jsonschema_description:"Location required for weather query"`
}

const weatherResponse = `{
 "location": %s,
 "timestamp": {
   "start": %d,
   "end": %d
 },
 "temperature": {
   "value": 20,
   "dataunit": "Celsius"
 },
 "condition": "Sunny",
 "humidity": 55,
 "wind": {
   "speed": 10,
   "dataunit": "km/h",
   "direction": "North-East"
 },
 "source": "WeatherAPI"
}
`

func newWeatherTool() tool.Tool {
	weatherTool, _ := tool.NewTool(
		tool.Definition{
			Name:        "weather_query",
			Description: "a weather query tool",
			InputSchema: pkgJson.StringDefSchemaOf(weatherRequest{}),
		},
		tool.Metadata{},
		func(_ *tool.Context, input string) (string, error) {
			fmt.Println(input)
			req := weatherRequest{}
			err := json.Unmarshal([]byte(input), &req)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(weatherResponse, req.Location, req.StartAt, req.EndAt), nil
		},
	)
	return weatherTool
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

func TestClient_Call_FC(t *testing.T) {
	client := newChatClient()
	text, err := client.
		ChatText("北京2025年5月1日天气情况").
		WithTools(newWeatherTool()).
		Call().
		Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
}

func TestClient_Stream_Chat(t *testing.T) {
	client := newChatClient()
	resp := client.Chat().Stream().Text(context.Background())
	for r, err := range resp {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(r)
	}
}

func TestClient_Stream_ChatText(t *testing.T) {
	client := newChatClient()
	resp := client.ChatText("Hi!,How are you!").Stream().Text(context.Background())
	for res, err := range resp {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(res)
	}
}

func TestClient_Stream_ChatRequest(t *testing.T) {
	client := newChatClient()
	chatReq, _ := chat.NewRequest([]messages.Message{messages.NewUserMessage("Hi!,How are you!")})
	res := client.ChatRequest(chatReq).Stream().Text(context.Background())
	for r, err := range res {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(r)
	}
}
