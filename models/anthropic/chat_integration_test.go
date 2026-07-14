//go:build integration

package anthropic_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func integrationModel(t *testing.T) *anthropic.ChatModel {
	t.Helper()
	key := testutil.RequireKey(t, "anthropic")
	modelID, _ := testutil.LookupEnv("LYNX_TEST_ANTHROPIC_MODEL")
	if modelID == "" {
		modelID = "claude-3-5-haiku-20241022"
	}
	opts, err := chat.NewOptions(modelID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := anthropic.NewChatModel(anthropic.ChatModelConfig{
		APIKey:         key,
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestChatModel_Call_Integration(t *testing.T) {
	m := integrationModel(t)
	req, _ := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Reply with the single word: pong"),
	})
	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	resp, err := m.Call(ctx, req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.JoinedText() == "" {
		t.Fatal("empty assistant text")
	}
	if !strings.Contains(strings.ToLower(resp.Result.AssistantMessage.JoinedText()), "pong") {
		t.Logf("note: model returned %q", resp.Result.AssistantMessage.JoinedText())
	}
}

func TestChatModel_Stream_Integration(t *testing.T) {
	m := integrationModel(t)
	req, _ := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Count from 1 to 5, comma-separated."),
	})
	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	resps, err := testutil.Collect(m.Stream(ctx, req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) < 2 {
		t.Fatalf("got %d chunks; want at least 2", len(resps))
	}
}
