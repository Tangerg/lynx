package openaiv2

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model/model"
)

var (
	baseURL   = "https://api.moonshot.cn/v1"
	baseModel = "moonshot-v1-8k"
)

func newApiKey() model.ApiKey {
	k := os.Getenv("apiKey")

	return model.NewApiKey(k)
}
func newApi() *Api {
	return NewApi(newApiKey(), option.WithBaseURL(baseURL))
}

func TestNewApi(t *testing.T) {
	api := newApi()
	t.Log(api)
}

func TestApi_ChatCompletion(t *testing.T) {
	api := newApi()
	completion, err := api.ChatCompletion(context.Background(), &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi!"),
		},
		Model: baseModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(completion.Choices[0].Message.Content)
}

func TestApi_ChatCompletionStream(t *testing.T) {
	api := newApi()
	stream := api.ChatCompletionStream(context.Background(), &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi!"),
		},
		Model: baseModel,
	})
	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		once := openai.ChatCompletionAccumulator{}
		once.AddChunk(chunk)
		content := once.Choices[0].Message.Content
		t.Log(content)
	}

	if stream.Err() != nil {
		t.Fatal(stream.Err())
	}

	content := acc.Choices[0].Message.Content
	t.Log(content)
}
