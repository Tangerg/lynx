package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/providers/models/openai"
	"github.com/Tangerg/lynx/pkg/assert"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
)

var (
	baseURL   = "https://api.moonshot.cn/v1"
	baseModel = "moonshot-v1-8k-vision-preview"
)

func newAPIKey() model.ApiKey {
	apiKey := os.Getenv("apiKey")

	return model.NewApiKey(apiKey)
}

func newChatModel() *openai.ChatModel {
	defaultOptions := assert.Must(chat.NewOptions(baseModel))

	return assert.Must(openai.NewChatModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func newChatClient() *chat.Client {
	req := assert.Must(
		chat.NewClientRequest(newChatModel()),
	)

	return assert.Must(
		chat.NewClient(req),
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
}`

func newWeatherTool() chat.Tool {
	toolDefinition := chat.ToolDefinition{
		Name:        "weather_query",
		Description: "a weather query internalTool",
		InputSchema: pkgJson.StringDefSchemaOf(weatherRequest{}),
	}

	toolMetadata := chat.ToolMetadata{}

	toolFunction := func(_ context.Context, input string) (string, error) {
		fmt.Println("weather_query called")
		fmt.Println(input)

		request := weatherRequest{}
		err := json.Unmarshal([]byte(input), &request)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf(
			weatherResponse,
			request.Location,
			request.StartAt,
			request.EndAt,
		), nil
	}

	weatherTool, _ := chat.NewTool(
		toolDefinition,
		toolMetadata,
		toolFunction,
	)

	return weatherTool
}

func TestClient_Call_Chat(t *testing.T) {
	client := newChatClient()

	text, _, err := client.
		Chat().
		Call().
		Text(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Log(text)
}

func TestClient_Call_ChatText(t *testing.T) {
	client := newChatClient()

	text, _, err := client.
		ChatText("Hi! How are you!").
		Call().
		Text(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Log(text)
}

func TestClient_Call_ChatText_List(t *testing.T) {
	client := newChatClient()

	colorList, _, err := client.
		ChatText("list five color").
		Call().
		List(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	for index, item := range colorList {
		t.Log(index, item)
	}
}

func TestClient_Call_ChatText_Map(t *testing.T) {
	client := newChatClient()

	userInfoMap, _, err := client.
		ChatText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information").
		Call().
		Map(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	for key, value := range userInfoMap {
		t.Log(key, value)
	}
}

func TestClient_Call_ChatText_Any(t *testing.T) {
	type user struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"Email"`
	}

	client := newChatClient()

	userAny, _, err := client.
		ChatText("Tom, 18 years old, Email is Tom@gmail.com. Please format this user's information").
		Call().
		Any(context.Background(), chat.JSONParserAsAnyOf[*user]())

	if err != nil {
		t.Fatal(err)
	}

	userInfo := userAny.(*user)
	t.Log(userInfo.Name, userInfo.Age, userInfo.Email)
}

func TestClient_Call_PromptTemplate(t *testing.T) {
	client := newChatClient()

	text, _, err := client.
		ChatPromptTemplate(
			chat.
				NewPromptTemplate().
				WithTemplate("Hi! My name is {{.name}}}, how are you?").
				WithVariable("name", "Tom"),
		).
		Call().
		Text(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Log(text)
}

func TestClient_Call_ChatRequest(t *testing.T) {
	client := newChatClient()

	messages := []chat.Message{
		chat.NewUserMessage("Hi!,How are you!"),
	}
	chatRequest, _ := chat.NewRequest(messages)

	text, _, err := client.
		ChatRequest(chatRequest).
		Call().
		Text(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Log(text)
}

func TestClient_Call_Tool(t *testing.T) {
	client := newChatClient()

	weatherTool := newWeatherTool()

	text, _, err := client.
		ChatText("I want to inquire about the weather conditions in Beijing on May 1st, 2023").
		WithTools(weatherTool).
		Call().
		Text(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Log(text)
}

func TestClient_Stream_Chat(t *testing.T) {
	client := newChatClient()

	responseStream := client.
		Chat().
		Stream().
		Text(context.Background())

	for response, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		t.Log(response)
	}
}

func TestClient_Stream_ChatText(t *testing.T) {
	client := newChatClient()

	responseStream := client.
		ChatText("Hi!,How are you!").
		Stream().
		Text(context.Background())

	for response, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		t.Log(response)
	}
}

func TestClient_Stream_PromptTemplate(t *testing.T) {
	client := newChatClient()

	responseStream := client.
		ChatPromptTemplate(
			chat.
				NewPromptTemplate().
				WithTemplate("Hi! My name is {{.name}}}, how are you?").
				WithVariable("name", "Tom"),
		).
		Stream().
		Text(context.Background())

	for response, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		t.Log(response)
	}
}

func TestClient_Stream_ChatRequest(t *testing.T) {
	client := newChatClient()

	messages := []chat.Message{
		chat.NewUserMessage("Hi!,How are you!"),
	}
	chatRequest, _ := chat.NewRequest(messages)

	responseStream := client.
		ChatRequest(chatRequest).
		Stream().
		Text(context.Background())

	for response, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		t.Log(response)
	}
}

func TestClient_Stream_Tool(t *testing.T) {
	client := newChatClient()

	weatherTool := newWeatherTool()

	responseStream := client.
		ChatText("I want to inquire about the weather conditions in Beijing on May 1st, 2023").
		WithTools(weatherTool).
		Stream().
		Text(context.Background())

	for response, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		t.Log(response)
	}
}
