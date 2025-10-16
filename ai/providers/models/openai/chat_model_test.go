package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/assert"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/pkg/mime"
)

func newChatModel() *ChatModel {
	defaultOptions := assert.Must(chat.NewOptions(baseModel))

	return assert.Must(NewChatModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func TestNewChatModel(t *testing.T) {
	chatModel := newChatModel()

	t.Log(chatModel)
}

func TestChatModel_Call(t *testing.T) {
	chatModel := newChatModel()

	messages := []chat.Message{
		chat.NewUserMessage("hi!"),
	}
	chatRequest, _ := chat.NewRequest(messages)

	response, err := chatModel.Call(context.Background(), chatRequest)
	if err != nil {
		t.Fatal(err)
	}

	responseText := response.
		Result().
		AssistantMessage.
		Text

	t.Log(responseText)
}

func TestChatModel_Stream(t *testing.T) {
	chatModel := newChatModel()

	messages := []chat.Message{
		chat.NewUserMessage("hi!"),
	}
	chatRequest, _ := chat.NewRequest(messages)

	responseStream := chatModel.Stream(context.Background(), chatRequest)

	for streamResponse, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		responseText := streamResponse.
			Result().
			AssistantMessage.
			Text

		t.Log(responseText)
	}
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
		Description: "a weather query tool",
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

func TestChatModel_Call_Tool(t *testing.T) {
	chatModel := newChatModel()

	weatherTool := newWeatherTool()
	toolOptions := assert.Must(chat.NewOptions(baseModel))
	toolOptions.Tools = []chat.Tool{weatherTool}
	messages := []chat.Message{
		chat.NewUserMessage("I want to inquire about the weather conditions in Beijing on May 1st, 2023"),
	}

	chatRequest, err := chat.NewRequest(messages)
	if err != nil {
		t.Fatal(err)
	}

	chatRequest.Options = toolOptions

	response, err := chatModel.Call(context.Background(), chatRequest)
	if err != nil {
		t.Fatal(err)
	}

	responseText := response.
		Result().
		AssistantMessage.
		Text

	t.Log(responseText)
}

func TestChatModel_Stream_Tool(t *testing.T) {
	chatModel := newChatModel()

	weatherTool := newWeatherTool()
	toolOptions := assert.Must(chat.NewOptions(baseModel))
	toolOptions.Tools = []chat.Tool{weatherTool}

	messages := []chat.Message{
		chat.NewUserMessage("I want to inquire about the weather conditions in Beijing on May 1st, 2023"),
	}

	chatRequest, err := chat.NewRequest(messages)
	if err != nil {
		t.Fatal(err)
	}

	chatRequest.Options = toolOptions

	responseStream := chatModel.Stream(context.Background(), chatRequest)

	for streamResponse, err := range responseStream {
		if err != nil {
			t.Fatal(err)
		}

		toolCalls := streamResponse.
			Result().
			AssistantMessage.
			ToolCalls

		for _, toolCall := range toolCalls {
			t.Log(toolCall.ID)
			t.Log(toolCall.Name)
			t.Log(toolCall.Arguments)
		}

		responseText := streamResponse.
			Result().
			AssistantMessage.
			Text

		t.Log(responseText)
	}
}

func TestChatModel_Call_Vision_Base64(t *testing.T) {
	chatModel := newChatModel()

	mediaContent, _ := media.NewMedia(mime.NewBuilder().
		WithType("image").
		WithSubType("png").
		MustBuild(), "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGAAAABhCAYAAAApxKSdAAAACXBIWXMAACE4AAAhOAFFljFgAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAUUSURBVHgB7Z29bhtHFIWPHQN2J7lKqnhYpYvpIukCbJEAKQJEegLReYFIT0DrCSI9QEDqCSIDaQIEIOukiJwyza5SJWlId3FFz+HuGmuSSw6p+dlZ3g84luhdUeI9M3fmziyXgBCUe/DHYY0Wj/tgWmjV42zFcWe4MIBBPNJ6qqW0uvAbXFvQgKzQK62bQhkaCIPc10q1Zi3XH1o/IG9cwUm0RogrgDY1KmLgHYX9DvyiBvDYI77XmiD+oLlQHw7hIDoCMBOt1U9w0BsU9mOAtaUUFk3oQoIfzAQFCf5dNMEdTFCQ4NtQih1NSIGgf3ibxOJt5UrAB1gNK72vIdjiI61HWr+YnNxDXK0rJiULsV65GJeiIuscLSTTeobKSutiCuojX8kU3MBx4I3WeNVBBRl4fWiCyoB8v2JAAkk9PmDwT8sH1TEghRjgC27scCx41wO43KAg+ILxTvhNaUACwTc04Z0B30LwzTzm5Rjw3sgseIG1wGMawMBPIOQcqvzrNIMHOg9Q5KK953O90/rFC+BhJRH8PQZ+fu7SjC7HAIV95yu99vjlxfvBJx8nwHd6IfNJAkccOjHg6OgIs9lsra6vr2GTNE03/k7q8HAhyJ/2gM9O65/4kT7/mwEcoZwYsPQiV3BwcABb9Ho9KKU2njccDjGdLlxx+InBBPBAAR86ydRPaIC9SASi3+8bnXd+fr78nw8NJ39uDJjXAVFPP7dp/VmWLR9g6w6Huo/IOTk5MTpvZesn/93AiP/dXCwd9SyILT9Jko3n1bZ+8s8rGPGvoVHbEXcPMM39V1dX9Qd/19PPNxta959D4HUGF0RrAFs/8/8mxuPxXLUwtfx2WX+cxdivZ3DFA0SKldZPuPTAKrikbOlMOX+9zFu/Q2iAQoSY5H7mfeb/tXCT8MdneU9wNNCuQUXZA0ynnrUznyqOcrspUY4BJunHqPU3gOgMsNr6G0B0BpgUXrG0fhKVAaaF1/HxMWIhKgNMcj9Tz82Nk6rVGdav/tJ5eraJ0Wi01XPq1r/xOS8uLkJc6XYnRTMNXdf62eIvLy+jyftVghnQ7Xahe8FW59fBTRYOzosDNI1hJdz0lBQkBflkMBjMU5iL13pXRb8fYAJrB/a2db0oFHthAOEUliaYFHE+aaUBdZsvvFhApyM0idYZwOCvW4JmIWdSzPmidQaYrAGZ7iX4oFUGnJ2dGdUCTRqMozeANQCLsE6nA10JG/0Mx4KmDMbBCjEWR2yxu8LAM98vXulmCA2ovVLCI8EMYODWbpbvCXtTBzQVMSAwYkBgxIDAtNKAXWdGIRADAiMpKDA0IIMQikx6QGDEgMCIAYGRMSAsMgaEhgbcQgjFa+kBYZnIGBCWWzEgLPNBOJ6Fk/aR8Y5ZCvktKwX/PJZ7xoVjfs+4chYU11tK2sE85qUBLyH4Zh5z6QHhGPOf6r2j+TEbcgdFP2RaHX5TrYQlDflj5RXE5Q1cG/lWnhYpReUGKdUewGnRmhvnCJbgmxey8sHiZ8iwF3AsUBBckKHI/SWLq6HsBc8huML4DiK80D6WnBqLzN68UFCmopheYJOVYgcU5FOVbAVfYUcUZGoaLPglCtITdg2+tZUFBTFh2+ArWEYh/7z0WIIQSiM43lt5AWAmWhLHylN4QmkNEXfAbGqEQKsHSfHLYwiSq8AnaAAKeaW3D8VbijwNW5nh3IN9FPI/jnpaPKZi2/SfFuJu4W3x9RqWL+N5C+7ruKpBAgLkAAAAAElFTkSuQmCC")

	messageParams := chat.MessageParams{
		Text:  "Please describe this picture",
		Media: []*media.Media{mediaContent},
	}

	messages := []chat.Message{
		chat.NewUserMessage(messageParams),
	}

	chatRequest, err := chat.NewRequest(messages)
	if err != nil {
		t.Fatal(err)
	}

	response, err := chatModel.Call(context.Background(), chatRequest)
	if err != nil {
		t.Fatal(err)
	}

	responseText := response.
		Result().
		AssistantMessage.
		Text

	t.Log(responseText)
}

func TestChatModel_Call_Vision_URL(t *testing.T) {
	chatModel := newChatModel()
	mediaContent, _ := media.NewMedia(mime.NewBuilder().
		WithType("image").
		WithSubType("png").
		MustBuild(), "https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/2560px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg")

	messageParams := chat.MessageParams{
		Text:  "Please describe this picture",
		Media: []*media.Media{mediaContent},
	}

	messages := []chat.Message{
		chat.NewUserMessage(messageParams),
	}

	chatRequest, err := chat.NewRequest(messages)
	if err != nil {
		t.Fatal(err)
	}

	response, err := chatModel.Call(context.Background(), chatRequest)
	if err != nil {
		t.Fatal(err)
	}

	responseText := response.
		Result().
		AssistantMessage.
		Text

	t.Log(responseText)
}
