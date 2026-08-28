package agentexec

import (
	"context"
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type interactionCountingModel struct{}

func (interactionCountingModel) Call(context.Context, *corechat.Request) (*corechat.Response, error) {
	return new(corechat.Response), nil
}

func (interactionCountingModel) CountInputTokens(context.Context, *corechat.Request) (int64, error) {
	return 41, nil
}

func TestObservedInteractionClientPreservesInputTokenCountingWithoutRecordingAModelCall(t *testing.T) {
	inner, err := chatclient.New(interactionCountingModel{}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := newObservedInteractionClient(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !observed.SupportsInputTokenCounting() {
		t.Fatal("observed client stripped input token counting")
	}
	request, _ := corechat.NewRequest(corechat.NewUserMessage(corechat.NewTextPart("hello")))
	count, err := observed.CountInputTokens(t.Context(), request)
	if err != nil || count != 41 {
		t.Fatalf("CountInputTokens = %d, %v; want 41, nil", count, err)
	}
}

func TestObservedInteractionClientDoesNotInventInputTokenCounting(t *testing.T) {
	inner, err := chatclient.New(corechat.ModelFunc(func(context.Context, *corechat.Request) (*corechat.Response, error) {
		return new(corechat.Response), nil
	}), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := newObservedInteractionClient(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observed.SupportsInputTokenCounting() {
		t.Fatal("observed client invented input token counting")
	}
}
