package openai

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model"
)

var (
	baseURL   = "https://api.moonshot.cn/v1"
	baseModel = "moonshot-v1-8k-vision-preview"
)

func newAPIKey() model.ApiKey {
	apiKey := os.Getenv("apiKey")

	return model.NewApiKey(apiKey)
}

func newAPI() *Api {
	apiInstance, _ := NewApi(
		newAPIKey(),
		option.WithBaseURL(baseURL),
	)

	return apiInstance
}

func TestNewApi(t *testing.T) {
	apiInstance := newAPI()

	t.Log(apiInstance)
}

func TestApi_ChatCompletion(t *testing.T) {
	apiInstance := newAPI()

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hi!"),
	}

	params := &openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    baseModel,
	}

	completion, err := apiInstance.ChatCompletion(
		context.Background(),
		params,
	)

	if err != nil {
		t.Fatal(err)
	}

	content := completion.Choices[0].Message.Content
	t.Log(content)
}

func TestApi_ChatCompletionStream(t *testing.T) {
	apiInstance := newAPI()

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hi!"),
	}

	params := &openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    baseModel,
	}

	streamResponse, err := apiInstance.ChatCompletionStream(
		context.Background(),
		params,
	)

	if err != nil {
		t.Fatal(err)
	}

	accumulator := openai.ChatCompletionAccumulator{}

	for streamResponse.Next() {
		chunk := streamResponse.Current()
		accumulator.AddChunk(chunk)

		chunkAccumulator := openai.ChatCompletionAccumulator{}
		chunkAccumulator.AddChunk(chunk)

		chunkContent := chunkAccumulator.Choices[0].Message.Content
		t.Log(chunkContent)
	}

	if streamResponse.Err() != nil {
		t.Fatal(streamResponse.Err())
	}

	finalContent := accumulator.Choices[0].Message.Content
	t.Log(finalContent)
}
