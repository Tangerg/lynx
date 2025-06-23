package openaiv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/assert"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
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
		messages.NewUserMessage("hi!", nil),
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
		messages.NewUserMessage("hi!", nil),
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
			messages.NewUserMessage("我想要查询北京在2023年5月1日的天气情况", nil),
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
