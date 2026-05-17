package azureopenai_test

import (
	"encoding/json"
	"net/http"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Call_Mock(t *testing.T) {
	completion := openaisdk.ChatCompletion{
		ID: "chatcmpl-az", Object: "chat.completion", Model: "gpt-4o-deployment",
		Choices: []openaisdk.ChatCompletionChoice{{
			Index: 0, FinishReason: "stop",
			Message: openaisdk.ChatCompletionMessage{Role: "assistant", Content: "azure says hi"},
		}},
		Usage: openaisdk.CompletionUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}
	body, _ := json.Marshal(completion)
	srv := testutil.JSONServer(http.StatusOK, string(body))
	t.Cleanup(srv.Close)

	opts, err := chat.NewOptions("gpt-4o-deployment")
	if err != nil {
		t.Fatal(err)
	}
	m, err := azureopenai.NewChatModel(&azureopenai.ChatModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		Endpoint:       srv.URL,
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.Text != "azure says hi" {
		t.Errorf("text = %q", resp.Result.AssistantMessage.Text)
	}
	if m.Metadata().Provider != azureopenai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, azureopenai.Provider)
	}
}
