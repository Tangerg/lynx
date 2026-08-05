package core_test

import (
	"context"
	"errors"
	"iter"
	"math"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

// stubModel returns a fixed reply (or error) for every Call.
type stubModel struct {
	reply string
	err   error

	gotPrompt string
}

func newStubModel(reply string) *stubModel {
	return &stubModel{reply: reply}
}

func newStubErrModel(err error) *stubModel {
	return &stubModel{err: err}
}

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	for index := range req.Messages {
		if req.Messages[index].Role == chat.RoleUser {
			m.gotPrompt = req.Messages[index].Text()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func managedConditionEnv(model chat.Model) *core.ConditionEnv {
	return &core.ConditionEnv{RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
		response, err := model.Call(ctx, input.Request)
		if err != nil {
			return interaction.Result{}, err
		}
		final := interaction.Event{Kind: interaction.EventModelResponse, Round: 1, Final: true, Response: response}
		return interaction.Result{Final: &final}, nil
	}}
}

func mustPromptCondition(t *testing.T, config core.PromptConditionConfig) *core.PromptCondition {
	t.Helper()
	condition, err := core.NewPromptCondition(config)
	if err != nil {
		t.Fatalf("NewPromptCondition: %v", err)
	}
	if condition == nil {
		t.Fatal("NewPromptCondition returned nil without an error")
	}
	return condition
}

func TestPromptCondition_YesReplyIsTrue(t *testing.T) {
	model := newStubModel("Yes, the draft is acceptable.")
	cond := mustPromptCondition(t, core.PromptConditionConfig{
		Name: "draft_acceptable",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string {
			return "Is this draft acceptable?"
		},
		Parse: core.ParseYesNo,
	})
	if cond.Name() != "draft_acceptable" {
		t.Fatalf("Name = %q, want draft_acceptable", cond.Name())
	}

	got := cond.Evaluate(t.Context(), managedConditionEnv(model))
	if got != core.True {
		t.Fatalf("Truth = %s, want True", got)
	}
	if model.gotPrompt != "Is this draft acceptable?" {
		t.Fatalf("prompt = %q, want %q", model.gotPrompt, "Is this draft acceptable?")
	}
}

func TestPromptCondition_NoReplyIsFalse(t *testing.T) {
	model := newStubModel("No.")
	cond := mustPromptCondition(t, core.PromptConditionConfig{
		Name:   "x",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "ok?" },
		Parse:  core.ParseYesNo,
	})

	if got := cond.Evaluate(t.Context(), managedConditionEnv(model)); got != core.False {
		t.Fatalf("Truth = %s, want False", got)
	}
}

func TestPromptCondition_AmbiguousReplyIsUnknown(t *testing.T) {
	model := newStubModel("Maybe, it depends.")
	cond := mustPromptCondition(t, core.PromptConditionConfig{
		Name:   "x",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "ok?" },
		Parse:  core.ParseYesNo,
	})

	if got := cond.Evaluate(t.Context(), managedConditionEnv(model)); got != core.Unknown {
		t.Fatalf("Truth = %s, want Unknown", got)
	}
}

func TestPromptCondition_LLMErrorDegradesToUnknown(t *testing.T) {
	model := newStubErrModel(errors.New("transient"))
	cond := mustPromptCondition(t, core.PromptConditionConfig{
		Name:   "x",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "ok?" },
		Parse:  core.ParseYesNo,
	})

	if got := cond.Evaluate(t.Context(), managedConditionEnv(model)); got != core.Unknown {
		t.Fatalf("LLM error → Truth = %s, want Unknown", got)
	}
}

func TestPromptCondition_CostDefaultsToOne(t *testing.T) {
	cond := mustPromptCondition(t, core.PromptConditionConfig{
		Name:   "x",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "ok?" },
		Parse:  core.ParseYesNo,
	})
	if cond.Cost() != 1 {
		t.Fatalf("Cost = %f, want 1", cond.Cost())
	}

	cond = mustPromptCondition(t, core.PromptConditionConfig{
		Name:   "x",
		Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "ok?" },
		Parse:  core.ParseYesNo,
		Cost:   2.5,
	})
	if cond.Cost() != 2.5 {
		t.Fatalf("Cost = %f, want 2.5", cond.Cost())
	}
}

func TestPromptCondition_RejectsInvalidArgs(t *testing.T) {
	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil prompt", func() error {
			_, err := core.NewPromptCondition(core.PromptConditionConfig{
				Name: "x", Parse: core.ParseYesNo,
			})
			return err
		}},
		{"nil parser", func() error {
			_, err := core.NewPromptCondition(core.PromptConditionConfig{
				Name: "x", Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "" },
			})
			return err
		}},
		{"name with surrounding whitespace", func() error {
			_, err := core.NewPromptCondition(core.PromptConditionConfig{
				Name: " x ", Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "" }, Parse: core.ParseYesNo,
			})
			return err
		}},
		{"NaN cost", func() error {
			_, err := core.NewPromptCondition(core.PromptConditionConfig{
				Name: "x", Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "" }, Parse: core.ParseYesNo, Cost: math.NaN(),
			})
			return err
		}},
		{"infinite cost", func() error {
			_, err := core.NewPromptCondition(core.PromptConditionConfig{
				Name: "x", Prompt: func(_ context.Context, _ *core.ConditionEnv) string { return "" }, Parse: core.ParseYesNo, Cost: math.Inf(1),
			})
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

func TestParseYesNo(t *testing.T) {
	cases := []struct {
		text string
		want core.Truth
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
			if got := core.ParseYesNo(tc.text); got != tc.want {
				t.Fatalf("ParseYesNo(%q) = %s, want %s", tc.text, got, tc.want)
			}
		})
	}
}
