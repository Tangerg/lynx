//go:build integration

package azureopenai_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Call_Integration(t *testing.T) {
	key := testutil.RequireKey(t, "azureopenai")
	endpoint := testutil.RequireEnv(t, "LYNX_TEST_AZUREOPENAI_ENDPOINT")
	deployment := testutil.RequireEnv(t, "LYNX_TEST_AZUREOPENAI_DEPLOYMENT")

	opts, err := chat.NewOptions(deployment)
	if err != nil {
		t.Fatal(err)
	}
	m, err := azureopenai.NewChatModel(&azureopenai.ChatModelConfig{
		APIKey:         model.NewAPIKey(key),
		Endpoint:       endpoint,
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("Reply: pong")})
	resp, err := m.Call(ctx, req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.JoinedText() == "" {
		t.Fatal("empty text")
	}
}
