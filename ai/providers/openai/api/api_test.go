package api

import (
	"context"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestNewOpenAIApi(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	t.Log(token)
	api := NewOpenAIApi(token)
	t.Log(api)
}

func TestAPICreateChatCompletion(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	api := NewOpenAIApi(token)
	response, err := api.CreateChatCompletion(context.Background(), &openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "hello! who are you?",
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	for _, choice := range response.Choices {
		t.Log(choice.Message.Content)
	}
}
