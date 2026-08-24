package utilitymodel

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

type recordingModel struct {
	request *chat.Request
}

func (model *recordingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.request = request
	message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func (model *recordingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := model.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func TestCompleteBuildsOneMiddlewareFreePrompt(t *testing.T) {
	model := &recordingModel{}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := Complete(t.Context(), client, "system instructions", "input")
	if err != nil {
		t.Fatal(err)
	}
	if text != "completed" {
		t.Fatalf("completion = %q, want completed", text)
	}
	if model.request == nil || len(model.request.Messages) != 2 {
		t.Fatalf("request messages = %#v, want system and user", model.request)
	}
	if model.request.Messages[0].Role != chat.RoleSystem || model.request.Messages[0].Text() != "system instructions" {
		t.Fatalf("system message = %#v", model.request.Messages[0])
	}
	if model.request.Messages[1].Role != chat.RoleUser || model.request.Messages[1].Text() != "input" {
		t.Fatalf("user message = %#v", model.request.Messages[1])
	}
	if model.request.Options.MaxTokens == nil || *model.request.Options.MaxTokens <= 0 {
		t.Fatalf("MaxTokens = %v, want an explicit positive auxiliary-output bound", model.request.Options.MaxTokens)
	}
}

func TestCompleteRejectsMissingClient(t *testing.T) {
	_, err := Complete(t.Context(), nil, "system", "input")
	if err == nil || err.Error() != "utilitymodel: client is required" {
		t.Fatalf("Complete nil client error = %v", err)
	}
}
