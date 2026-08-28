package chatclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type countingModel struct {
	counted *chat.Request
}

type inputTokenCounterFunc func(context.Context, *chat.Request) (int64, error)

func (i inputTokenCounterFunc) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	return i(ctx, request)
}

func (*countingModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return new(chat.Response), nil
}

func (c *countingModel) CountInputTokens(_ context.Context, request *chat.Request) (int64, error) {
	c.counted = request
	return 37, nil
}

func TestClientCountsPreparedInputWithoutMutatingCaller(t *testing.T) {
	model := new(countingModel)
	client, err := chatclient.New(model, chatclient.Config{
		Defaults: chat.Options{Model: "provider-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatal(err)
	}
	count, err := client.CountInputTokens(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if count != 37 || model.counted == nil || model.counted.Options.Model != "provider-model" {
		t.Fatalf("count = %d request = %#v", count, model.counted)
	}
	if request.Options.Model != "" || model.counted == request {
		t.Fatal("counting mutated or retained the caller-owned request")
	}
}

func TestClientDoesNotAdvertiseCountingAcrossCallMiddleware(t *testing.T) {
	client, err := chatclient.New(new(countingModel), chatclient.Config{
		CallMiddleware: []chat.CallMiddleware{func(next chat.Model) chat.Model {
			return chat.ModelFunc(next.Call)
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.SupportsInputTokenCounting() {
		t.Fatal("client advertised counting after a call middleware could change the provider request")
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if _, err := client.CountInputTokens(t.Context(), request); !errors.Is(err, chatclient.ErrInputTokenCountingUnsupported) {
		t.Fatalf("CountInputTokens error = %v", err)
	}
}

func TestClientRejectsNegativeInputTokenCount(t *testing.T) {
	model := struct {
		chat.Model
		inputTokenCounterFunc
	}{
		Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return new(chat.Response), nil
		}),
		inputTokenCounterFunc: inputTokenCounterFunc(func(context.Context, *chat.Request) (int64, error) {
			return -1, nil
		}),
	}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if _, err := client.CountInputTokens(t.Context(), request); err == nil {
		t.Fatal("CountInputTokens accepted a negative provider count")
	}
}
