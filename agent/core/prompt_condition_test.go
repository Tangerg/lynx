package core_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// stubModel returns a fixed reply (or error) for every Call.
type stubModel struct {
	defaults *chat.Options
	reply    string
	err      error

	gotPrompt string
}

func newStubModel(reply string) *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{defaults: opts, reply: reply}
}

func newStubErrModel(err error) *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{defaults: opts, err: err}
}

func (m *stubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *stubModel) Metadata() chat.ModelMetadata          { return chat.ModelMetadata{Provider: "stub"} }

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	for _, msg := range req.Messages {
		if msg.Type() == chat.MessageTypeUser {
			if u, ok := msg.(*chat.UserMessage); ok {
				m.gotPrompt = u.Text
			}
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.reply),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func newStubChatClient(t *testing.T, model chat.Model) *chat.Client {
	t.Helper()
	client, err := chat.NewClient(model)
	if err != nil {
		t.Fatalf("NewClientWithModel: %v", err)
	}
	return client
}

func TestPromptCondition_YesReplyIsTrue(t *testing.T) {
	model := newStubModel("Yes, the draft is acceptable.")
	cond, _ := core.NewPromptCondition(
		"draft_acceptable",
		newStubChatClient(t, model),
		func(_ context.Context, _ *core.OperationContext) string {
			return "Is this draft acceptable?"
		},
		core.ParseYesNoDetermination,
	)

	got := cond.Evaluate(t.Context(), &core.OperationContext{})
	if got != core.True {
		t.Fatalf("Determination = %s, want True", got)
	}
	if model.gotPrompt != "Is this draft acceptable?" {
		t.Fatalf("prompt = %q, want %q", model.gotPrompt, "Is this draft acceptable?")
	}
}

func TestPromptCondition_NoReplyIsFalse(t *testing.T) {
	model := newStubModel("No.")
	cond, _ := core.NewPromptCondition(
		"x",
		newStubChatClient(t, model),
		func(_ context.Context, _ *core.OperationContext) string { return "ok?" },
		core.ParseYesNoDetermination,
	)

	if got := cond.Evaluate(t.Context(), &core.OperationContext{}); got != core.False {
		t.Fatalf("Determination = %s, want False", got)
	}
}

func TestPromptCondition_AmbiguousReplyIsUnknown(t *testing.T) {
	model := newStubModel("Maybe, it depends.")
	cond, _ := core.NewPromptCondition(
		"x",
		newStubChatClient(t, model),
		func(_ context.Context, _ *core.OperationContext) string { return "ok?" },
		core.ParseYesNoDetermination,
	)

	if got := cond.Evaluate(t.Context(), &core.OperationContext{}); got != core.Unknown {
		t.Fatalf("Determination = %s, want Unknown", got)
	}
}

func TestPromptCondition_LLMErrorDegradesToUnknown(t *testing.T) {
	model := newStubErrModel(errors.New("transient"))
	cond, _ := core.NewPromptCondition(
		"x",
		newStubChatClient(t, model),
		func(_ context.Context, _ *core.OperationContext) string { return "ok?" },
		core.ParseYesNoDetermination,
	)

	if got := cond.Evaluate(t.Context(), &core.OperationContext{}); got != core.Unknown {
		t.Fatalf("LLM error → Determination = %s, want Unknown", got)
	}
}

func TestPromptCondition_CostDefaultsToOne(t *testing.T) {
	cond, _ := core.NewPromptCondition(
		"x",
		newStubChatClient(t, newStubModel("yes")),
		func(_ context.Context, _ *core.OperationContext) string { return "ok?" },
		core.ParseYesNoDetermination,
	)
	if cond.Cost() != 1.0 {
		t.Fatalf("default Cost = %f, want 1.0", cond.Cost())
	}
	cond.WithCost(2.5)
	if cond.Cost() != 2.5 {
		t.Fatalf("WithCost(2.5).Cost() = %f", cond.Cost())
	}
}

func TestPromptCondition_RejectsInvalidArgs(t *testing.T) {
	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil client", func() error {
			_, err := core.NewPromptCondition("x", nil,
				func(_ context.Context, _ *core.OperationContext) string { return "" },
				core.ParseYesNoDetermination)
			return err
		}},
		{"nil prompt", func() error {
			_, err := core.NewPromptCondition("x", newStubChatClient(t, newStubModel("yes")),
				nil, core.ParseYesNoDetermination)
			return err
		}},
		{"nil parser", func() error {
			_, err := core.NewPromptCondition("x", newStubChatClient(t, newStubModel("yes")),
				func(_ context.Context, _ *core.OperationContext) string { return "" }, nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseYesNoDetermination(t *testing.T) {
	cases := []struct {
		text string
		want core.Determination
	}{
		{"yes", core.True},
		{"Yes", core.True},
		{"Yes, definitely", core.True},
		{"YES.", core.True},
		{"true", core.True},
		{"y", core.True},
		{"affirmative", core.True},
		{"correct", core.True},
		{"1", core.True},
		{"no", core.False},
		{"No", core.False},
		{"NO!", core.False},
		{"false", core.False},
		{"n", core.False},
		{"negative", core.False},
		{"incorrect", core.False},
		{"0", core.False},
		{"maybe", core.Unknown},
		{"it depends", core.Unknown},
		{"", core.Unknown},
		{"   ", core.Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := core.ParseYesNoDetermination(tc.text); got != tc.want {
				t.Fatalf("ParseYesNoDetermination(%q) = %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}
