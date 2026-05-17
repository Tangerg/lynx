//go:build integration

package openai_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

// integrationModel returns a real OpenAI client for end-to-end probes.
// Override LYNX_TEST_OPENAI_MODEL to target a specific model.
func integrationModel(t *testing.T) *openai.ChatModel {
	t.Helper()
	key := testutil.RequireKey(t, "openai")
	modelID, _ := testutil.LookupEnv("LYNX_TEST_OPENAI_MODEL")
	if modelID == "" {
		modelID = "gpt-4o-mini"
	}
	opts, err := chat.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := openai.NewChatModel(&openai.ChatModelConfig{
		ApiKey:         model.NewApiKey(key),
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	return m
}

func TestChatModel_Call_Integration(t *testing.T) {
	m := integrationModel(t)
	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Reply with the single word: pong"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	resp, err := m.Call(ctx, req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.Text == "" {
		t.Fatal("empty assistant text")
	}
	if !strings.Contains(strings.ToLower(resp.Result.AssistantMessage.Text), "pong") {
		t.Logf("note: model returned %q (expected to contain 'pong')", resp.Result.AssistantMessage.Text)
	}
	if resp.Metadata.Usage == nil || resp.Metadata.Usage.TotalTokens() == 0 {
		t.Error("usage missing")
	}
}

func TestChatModel_Stream_Integration(t *testing.T) {
	m := integrationModel(t)
	req, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Count from 1 to 5, comma-separated."),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	resps, err := testutil.Collect(m.Stream(ctx, req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) < 2 {
		t.Fatalf("got %d chunks; expected at least 2", len(resps))
	}
}
