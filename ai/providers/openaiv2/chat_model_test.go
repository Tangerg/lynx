package openaiv2

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/pkg/assert"
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
