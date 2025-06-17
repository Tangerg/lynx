package api

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go"

	"github.com/Tangerg/lynx/ai/model/model"
)

func getApiKey() model.ApiKey {
	openAIkey := os.Getenv("openai_key")
	return model.NewApiKey(openAIkey)
}
func TestNewOpenAIApi(t *testing.T) {
	client := NewOpenAIApi(getApiKey())
	t.Log(client)
}

func TestOpenAIApi_ChatCompletion(t *testing.T) {
	client := NewOpenAIApi(getApiKey())
	completion, err := client.ChatCompletion(context.Background(), &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hi!"),
		},
		Model: openai.ChatModelGPT4o,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(completion.Choices[0].Message.Content)
}

func TestOpenAIApi_ChatCompletionStream(t *testing.T) {
	client := NewOpenAIApi(getApiKey())
	stream := client.ChatCompletionStream(context.Background(), &openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hi!"),
		},
		Model: openai.ChatModelGPT4o,
	})

	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if content, ok := acc.JustFinishedContent(); ok {
			t.Log("Content stream finished:", content)
		}

		if tool, ok := acc.JustFinishedToolCall(); ok {
			t.Log("Tool call stream finished:", tool.Index, tool.Name, tool.Arguments)
		}

		if refusal, ok := acc.JustFinishedRefusal(); ok {
			t.Log("Refusal stream finished:", refusal)
		}

		if len(chunk.Choices) > 0 {
			t.Log(chunk.Choices[0].Delta.Content)
		}
	}

	if stream.Err() != nil {
		t.Log(stream.Err())
	}

	_ = acc.Choices[0].Message.Content
}
