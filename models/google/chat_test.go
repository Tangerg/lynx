package google_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

// genai response shape: candidates[0].content.parts[0].text.
const googleChatJSON = `{
  "candidates": [{
    "content": {"role": "model", "parts": [{"text": "hello back"}]},
    "finishReason": "STOP"
  }],
  "usageMetadata": {"promptTokenCount": 4, "candidatesTokenCount": 2, "totalTokenCount": 6},
  "modelVersion": "gemini-2.0-flash"
}`

func TestChatModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, googleChatJSON)
	t.Cleanup(srv.Close)

	opts, err := chat.NewOptions("gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewChatModel(&google.ChatModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello back" {
		t.Errorf("text = %q; want 'hello back'", resp.Result.AssistantMessage.JoinedText())
	}
	if m.Metadata().Provider != google.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
