package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/commons/content"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/assert"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/pkg/mime"
)

func newChatModel() *ChatModel {
	defaultOptions := NewChatOptionsBuilder().WithModel(baseModel).MustBuild()
	return assert.ErrorIsNil(NewChatModel(newApiKey(), defaultOptions, option.WithBaseURL(baseURL)))
}

func TestNewChatModel(t *testing.T) {
	newModel := newChatModel()
	t.Log(newModel)
}

func TestChatModel_Call(t *testing.T) {
	newModel := newChatModel()
	request, _ := chat.NewRequest([]messages.Message{
		messages.NewUserMessage("hi!"),
	})
	resp, err := newModel.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp.Result().Output().Text())
}

func TestChatModel_Stream(t *testing.T) {
	newModel := newChatModel()
	request, _ := chat.NewRequest([]messages.Message{
		messages.NewUserMessage("hi!"),
	})
	reader, err := newModel.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	for {
		read, err := reader.Read(context.Background())
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		response, err := read.Get()
		if err != nil {
			t.Fatal(err)
		}
		t.Log(response.Result().Output().Text())
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
}
`

func newWeatherTool() tool.Tool {
	def := tool.
		NewDefinitionBuilder().
		WithName("weather_query").
		WithDescription("a weather query tool").
		WithInputSchema(pkgJson.StringDefSchemaOf(weatherRequest{})).
		MustBuild()

	weatherTool := tool.
		NewBuilder().
		WithDefinition(def).
		WithCaller(
			func(_ tool.Context, input string) (string, error) {
				fmt.Println(input)
				req := weatherRequest{}
				err := json.Unmarshal([]byte(input), &req)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf(weatherResponse, req.Location, req.StartAt, req.EndAt), nil
			},
		).
		MustBuild()
	return weatherTool
}

func TestChatModel_Call_Tool(t *testing.T) {
	newModel := newChatModel()
	opts := NewChatOptionsBuilder().
		WithTools([]tool.Tool{newWeatherTool()}).
		WithModel(baseModel).
		MustBuild()
	request, err := chat.NewRequest(
		[]messages.Message{
			messages.NewUserMessage("我想要查询北京在2023年5月1日的天气情况"),
		},
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newModel.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp.Result().Output().Text())
}

func TestChatModel_Stream_Tool(t *testing.T) {
	newModel := newChatModel()
	opts := NewChatOptionsBuilder().
		WithTools([]tool.Tool{newWeatherTool()}).
		WithModel(baseModel).
		MustBuild()
	request, err := chat.NewRequest(
		[]messages.Message{
			messages.NewUserMessage("我想要查询北京在2023年5月1日的天气情况"),
		},
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := newModel.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for {
		read, err := reader.Read(context.Background())
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		response, err := read.Get()
		if err != nil {
			t.Fatal(err)
		}
		for _, toolCall := range response.Result().Output().ToolCalls() {
			t.Log(toolCall.ID)
			t.Log(toolCall.Name)
			t.Log(toolCall.Arguments)
		}
		t.Log(response.Result().Output().Text())
	}
}

func TestChatModel_Call_Vision_Base64(t *testing.T) {
	newModel := newChatModel()

	request, err := chat.NewRequest(
		[]messages.Message{
			messages.NewUserMessage(messages.UserMessageParam{
				Text: "请描述这个图片",
				Media: []*content.Media{
					content.
						NewMediaBuilder().
						WithMimeType(
							mime.
								NewBuilder().
								WithType("image").
								WithSubType("png").
								MustBuild(),
						).
						WithData("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGAAAABhCAYAAAApxKSdAAAACXBIWXMAACE4AAAhOAFFljFgAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAUUSURBVHgB7Z29bhtHFIWPHQN2J7lKqnhYpYvpIukCbJEAKQJEegLReYFIT0DrCSI9QEDqCSIDaQIEIOukiJwyza5SJWlId3FFz+HuGmuSSw6p+dlZ3g84luhdUeI9M3fmziyXgBCUe/DHYY0Wj/tgWmjV42zFcWe4MIBBPNJ6qqW0uvAbXFvQgKzQK62bQhkaCIPc10q1Zi3XH1o/IG9cwUm0RogrgDY1KmLgHYX9DvyiBvDYI77XmiD+oLlQHw7hIDoCMBOt1U9w0BsU9mOAtaUUFk3oQoIfzAQFCf5dNMEdTFCQ4NtQih1NSIGgf3ibxOJt5UrAB1gNK72vIdjiI61HWr+YnNxDXK0rJiULsV65GJeiIescLSTTeobKSutiCuojX8kU3MBx4I3WeNVBBRl4fWiCyoB8v2JAAkk9PmDwT8sH1TEghRjgC27scCx41wO43KAg+ILxTvhNaUACwTc04Z0B30LwzTzm5Rjw3sgseIG1wGMawMBPIOQcqvzrNIMHOg9Q5KK953O90/rFC+BhJRH8PQZ+fu7SjC7HAIV95yu99vjlxfvBJx8nwHd6IfNJAkccOjHg6OgIs9lsra6vr2GTNE03/k7q8HAhyJ/2gM9O65/4kT7/mwEcoZwYsPQiV3BwcABb9Ho9KKU2njccDjGdLlxx+InBBPBAAR86ydRPaIC9SASi3+8bnXd+fr78nw8NJ39uDJjXAVFPP7dp/VmWLR9g6w6Huo/IOTk5MTpvZesn/93AiP/dXCwd9SyILT9Jko3n1bZ+8s8rGPGvoVHbEXcPMM39V1dX9Qd/19PPNxta959D4HUGF0RrAFs/8/8mxuPxXLUwtfx2WX+cxdivZ3DFA0SKldZPuPTAKrikbOlMOX+9zFu/Q2iAQoSY5H7mfeb/tXCT8MdneU9wNNCuQUXZA0ynnrUznyqOcrspUY4BJunHqPU3gOgMsNr6G0B0BpgUXrG0fhKVAaaF1/HxMWIhKgNMcj9Tz82Nk6rVGdav/tJ5eraJ0Wi01XPq1r/xOS8uLkJc6XYnRTMNXdf62eIvLy+jyftVghnQ7Xahe8FW59fBTRYOzosDNI1hJdz0lBQkBflkMBjMU5iL13pXRb8fYAJrB/a2db0oFHthAOEUliaYFHE+aaUBdZsvvFhApyM0idYZwOCvW4JmIWdSzPmidQaYrAGZ7iX4oFUGnJ2dGdUCTRqMozeANQCLsE6nA10JG/0Mx4KmDMbBCjEWR2yxu8LAM98vXelmCA2ovVLCI8EMYODWbpbvCXtTBzQVMSAwYkBgxIDAtNKAXWdGIRADAiMpKDA0IIMQikx6QGDEgMCIAYGRMSAsMgaEhgbcQgjFa+kBYZnIGBCWWzEgLPNBOJ6Fk/aR8Y5ZCvktKwX/PJZ7xoVjfs+4chYU11tK2sE85qUBLyH4Zh5z6QHhGPOf6r2j+TEbcgdFP2RaHX5TrYQlDflj5RXE5Q1cG/lWnhYpReUGKdUewGnRmhvnCJbgmxey8sHiZ8iwF3AsUBBckKHI/SWLq6HsBc8huML4DiK80D6WnBqLzN68UFCmopheYJOVYgcU5FOVbAVfYUcUZGoaLPglCtITdg2+tZUFBTFh2+ArWEYh/7z0WIIQSiM43lt5AWAmWhLHylN4QmkNEXfAbGqEQKsHSfHLYwiSq8AnaAAKeaW3D8VbijwNW5nh3IN9FPI/jnpaPKZi2/SfFuJu4W3x9RqWL+N5C+7ruKpBAgLkAAAAAElFTkSuQmCC").MustBuild(),
				},
			}),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newModel.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp.Result().Output().Text())
}

func TestChatModel_Call_Vision_URL(t *testing.T) {
	newModel := newChatModel()

	request, err := chat.NewRequest(
		[]messages.Message{
			messages.NewUserMessage(messages.UserMessageParam{
				Text: "请描述这个图片",
				Media: []*content.Media{
					content.
						NewMediaBuilder().
						WithMimeType(
							mime.
								NewBuilder().
								WithType("image").
								WithSubType("png").
								MustBuild(),
						).
						WithData("https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/2560px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg").MustBuild(),
				},
			}),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newModel.Call(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp.Result().Output().Text())
}
