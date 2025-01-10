package openai

import (
	"context"
	"encoding/json"
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
	"github.com/Tangerg/lynx/ai/core/chat/function"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"github.com/Tangerg/lynx/ai/providers/openai/chat"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
)

type weatherRequest struct {
	StartAt  int64  `json:"after" jsonschema_description:"Start time of weather query, second level timestamp in Unix format"`
	EndAt    int64  `json:"before" jsonschema_description:"End time of weather query, second level timestamp in Unix format"`
	Location string `json:"location" jsonschema_description:"Location required for weather query"`
}

const result = `{
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

func newClient2() client.ChatClient[O, M] {
	token := os.Getenv("OPENAI_TOKEN")
	openAIChatModel := chat.NewOpenAIChatModel(api.NewOpenAIApi(token), nil)
	sg := safeguard.New[O, M]("fuck")

	cm := chatmemory.New[O, M](&chatmemory.Config{
		Store: chatMemory,
	})
	og := outputguide.New[O, M]()
	tr := templaterender.New[O, M]()
	mi := modelinvoke.New[O, M]()

	func1, _ := function.
		NewWrapperBuilder().
		WithName("global_weather_query").
		WithDescription("A tool to query weather for weather conditions at any location and time period worldwide since 2000").
		WithInputTypeSchema(pkgJson.StringDefSchemaOf(weatherRequest{})).
		WithCaller(func(ctx context.Context, input string) (string, error) {
			fmt.Println(input)
			req := weatherRequest{}
			err := json.Unmarshal([]byte(input), &req)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(result, req.Location, req.StartAt, req.EndAt), nil
		}).
		Build()

	opts, _ := chat.
		NewOpenAIChatRequestOptionsBuilder().
		WithModel(openai.GPT4oMini).
		WithFunctions(func1).
		Build()

	cli := client.
		NewDefaultChatClientBuilder(openAIChatModel).
		DefaultChatRequestOptions(opts).
		DefaultMiddlewaresWithParams(
			map[string]any{},
			sg, cm, og, tr, mi,
		).
		Build()
	return cli
}

func TestFunctionCall(t *testing.T) {
	cli := newClient2()
	_, err := cli.PromptText("I would like to know the weather conditions in Beijing on May 16th, 2021").
		Call().
		Content(context.Background())
	if err != nil {
		t.Log(err)
		return
	}
	msgs, _ := chatMemory.Get(context.Background(), "", 1000)
	for _, msg := range msgs {
		t.Log(msg.Type()+":", msg.Content())
	}
}
