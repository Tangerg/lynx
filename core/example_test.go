package core_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// echoModel stands in for a provider so the overview stays runnable. A real
// implementation lives in its own models/<provider> module.
type echoModel struct{}

func (echoModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart(request.Messages[0].Text()))
	output, err := chat.NewOutput(&message, chat.FinishReasonStop, nil)
	if err != nil {
		return nil, err
	}
	return chat.NewResponse(output, nil)
}

// Example shows the ordinary path through the module: build a protocol request,
// wrap a provider model in a client that owns the defaults, and read the
// response through the protocol value rather than a provider type.
func Example() {
	client, err := chatclient.New(echoModel{}, chatclient.Config{
		Defaults: chat.Options{Model: "example-model"},
	})
	if err != nil {
		panic(err)
	}

	request := &chat.Request{
		Messages: []chat.Message{
			chat.NewUserMessage(chat.NewTextPart("hello")),
		},
	}
	response, err := client.Call(context.Background(), request)
	if err != nil {
		panic(err)
	}

	fmt.Println(response.Text())
	// Output:
	// hello
}
