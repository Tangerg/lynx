package openai

import (
	"context"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model/media"
	"github.com/Tangerg/lynx/ai/models/openai/chat"
)

const imageURL = "https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/2560px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg"

func TestChatWithMedia(t *testing.T) {
	client := newClient()
	options, _ := chat.
		NewOpenAIChatOptionsBuilder().
		WithModel(openai.GPT4o20240806).
		Build()

	p, err := prompt.NewChatPromptBuilder[O]().
		WithMessages(message.NewUserMessage("What’s in this image?",
			nil,
			media.New(nil,
				[]byte(imageURL)))).
		WithOptions(options).
		Build()
	if err != nil {
		t.Fatal(err)
		return
	}
	content, err := client.
		PromptPrompt(p).
		Call().
		Content(context.Background())
	if err != nil {
		t.Fatal(err)
		return
	}
	t.Log(content)
}