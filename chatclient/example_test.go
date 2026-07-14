package chatclient_test

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func ExampleClient_Call() {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return textResponse("Hello from the model"), nil
	})
	client, err := chatclient.New(model, chatclient.WithDefaults(chat.Options{Model: "example"}))
	if err != nil {
		panic(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("Hello")))
	if err != nil {
		panic(err)
	}
	response, err := client.Call(context.Background(), request)
	if err != nil {
		panic(err)
	}
	fmt.Println(response.Text())
	// Output: Hello from the model
}

func ExampleClient_Stream() {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return textResponse("fallback"), nil
	})
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			if !yield(response("Hello ", ""), nil) {
				return
			}
			yield(response("stream", chat.FinishReasonStop), nil)
		}
	})
	client, err := chatclient.New(model, chatclient.WithStreamer(streamer))
	if err != nil {
		panic(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("Hello")))
	if err != nil {
		panic(err)
	}
	for response, streamErr := range client.Stream(context.Background(), request) {
		if streamErr != nil {
			panic(streamErr)
		}
		fmt.Print(response.Text())
	}
	fmt.Println()
	// Output: Hello stream
}

func ExampleTemplate() {
	prompt, err := chatclient.ParseTemplate("Explain {{.Topic}} in one sentence.")
	if err != nil {
		panic(err)
	}
	message, err := prompt.UserMessage(struct{ Topic string }{Topic: "Go interfaces"})
	if err != nil {
		panic(err)
	}
	fmt.Println(message.Text())
	// Output: Explain Go interfaces in one sentence.
}

func ExampleCallStructured() {
	type answer struct {
		Value int `json:"value"`
	}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return textResponse(`{"value":42}`), nil
	})
	client, err := chatclient.New(model)
	if err != nil {
		panic(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("What is six times seven?")))
	if err != nil {
		panic(err)
	}
	result, _, err := chatclient.CallStructured(context.Background(), client, request, chatclient.JSON[answer]())
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Value)
	// Output: 42
}

func textResponse(text string) *chat.Response {
	return response(text, chat.FinishReasonStop)
}

func response(text string, reason chat.FinishReason) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Choices: []chat.Choice{{
		Index:        0,
		Message:      &message,
		FinishReason: reason,
	}}}
}
