package utilitymodel

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type recordingModel struct {
	request *chat.Request
}

func (r *recordingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	r.request = request
	message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (r *recordingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := r.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func TestCompleteBuildsOneMiddlewareFreePrompt(t *testing.T) {
	model := &recordingModel{}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := Complete(t.Context(), client, Prompt{
		SystemPrompt: "system instructions", UserPrompt: "input",
		MaxInputBytes: 1024, MaxOutputTokens: 123,
	})
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
	if model.request.Options.MaxTokens == nil || *model.request.Options.MaxTokens != 123 {
		t.Fatalf("MaxTokens = %v, want 123", model.request.Options.MaxTokens)
	}
}

func TestCompleteRejectsMissingClient(t *testing.T) {
	_, err := Complete(t.Context(), nil, Prompt{MaxInputBytes: 1, MaxOutputTokens: 1})
	if err == nil || err.Error() != "utilitymodel: client is required" {
		t.Fatalf("Complete nil client error = %v", err)
	}
}

func TestCompleteRejectsInvalidResourceEnvelopeBeforeCallingModel(t *testing.T) {
	model := &recordingModel{}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []Prompt{
		{MaxOutputTokens: 1},
		{MaxInputBytes: 1},
		{SystemPrompt: "system", UserPrompt: "input", MaxInputBytes: 5, MaxOutputTokens: 1},
	} {
		if _, err := Complete(t.Context(), client, prompt); err == nil {
			t.Fatalf("Complete(%+v) succeeded, want invalid resource envelope", prompt)
		}
	}
	if model.request != nil {
		t.Fatal("invalid resource envelope reached the model")
	}
}
