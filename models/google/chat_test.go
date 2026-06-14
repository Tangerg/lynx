package google_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/pkg/mime"
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
	m, err := google.NewChatModel(google.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
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

// TestChatModel_Call_ImageMedia_InlineData guards the image-input lowering: a
// UserMessage carrying base64-STRING image media must reach the wire as an
// inline-data part. The genai SDK wants raw []byte (it base64-encodes itself),
// so passing the base64 string straight to DataAsBytes failed the type
// assertion and the image was silently dropped — the bug this test pins.
func TestChatModel_Call_ImageMedia_InlineData(t *testing.T) {
	const b64 = "iVBORw0KGgoAAAANSUhEUg=="

	var seenBody []byte
	srv := testutil.JSONServer(http.StatusOK, googleChatJSON, func(r *http.Request) {
		seenBody, _ = io.ReadAll(r.Body)
	})
	t.Cleanup(srv.Close)

	opts, err := chat.NewOptions("gemini-2.0-flash")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewChatModel(google.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	img, err := media.NewMedia(mime.MustNew("image", "png"), b64)
	if err != nil {
		t.Fatalf("NewMedia: %v", err)
	}
	msg := chat.NewUserMessage(chat.MessageParams{Text: "what is this", Media: []*media.Media{img}})
	req, err := chat.NewRequest([]chat.Message{msg})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := m.Call(t.Context(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}

	// genai re-encodes the decoded bytes to base64 on the wire, so the original
	// base64 round-trips into the inline-data part. Its presence proves the
	// image wasn't dropped; the mime confirms the part type.
	body := string(seenBody)
	if !strings.Contains(body, b64) {
		t.Errorf("request body missing image base64 — image was dropped; body=%s", body)
	}
	if !strings.Contains(body, "image/png") {
		t.Errorf("request body missing image/png mime type")
	}
}
