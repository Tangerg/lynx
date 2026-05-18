package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func newChatModel(t *testing.T, baseURL, modelID string) *openai.ChatModel {
	t.Helper()
	opts, err := chat.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := openai.NewChatModel(&openai.ChatModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	return m
}

func TestChatModel_Call_Mock(t *testing.T) {
	completion := openaisdk.ChatCompletion{
		ID:      "chatcmpl-abc",
		Object:  "chat.completion",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []openaisdk.ChatCompletionChoice{{
			Index:        0,
			FinishReason: "stop",
			Message: openaisdk.ChatCompletionMessage{
				Role:    "assistant",
				Content: "hello world",
			},
		}},
		Usage: openaisdk.CompletionUsage{
			PromptTokens:     5,
			CompletionTokens: 2,
			TotalTokens:      7,
		},
	}
	body, _ := json.Marshal(completion)

	var seenURL, seenAuth, seenBodyModel string
	srv := testutil.JSONServer(http.StatusOK, string(body), func(r *http.Request) {
		seenURL = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		var probe struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(raw, &probe)
		seenBodyModel = probe.Model
	})
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "gpt-4o")
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if !strings.HasSuffix(seenURL, "/chat/completions") {
		t.Errorf("URL = %q; want suffix /chat/completions", seenURL)
	}
	if seenAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q; want Bearer test-key", seenAuth)
	}
	if seenBodyModel != "gpt-4o" {
		t.Errorf("body model = %q; want gpt-4o", seenBodyModel)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello world" {
		t.Errorf("assistant text = %q; want %q", resp.Result.AssistantMessage.JoinedText(), "hello world")
	}
	if resp.Metadata.Usage.PromptTokens != 5 || resp.Metadata.Usage.CompletionTokens != 2 {
		t.Errorf("usage = %+v; want 5/2", resp.Metadata.Usage)
	}
}

func TestChatModel_Stream_Mock(t *testing.T) {
	chunks := []string{
		`{"id":"x","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"id":"x","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"id":"x","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"id":"x","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
	}
	srv := testutil.OpenAISSEServer(chunks)
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "gpt-4o")
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resps, err := testutil.Collect(m.Stream(t.Context(), req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) != len(chunks) {
		t.Fatalf("got %d chunks, want %d", len(resps), len(chunks))
	}

	var pieces []string
	for _, r := range resps {
		if r.Result != nil && r.Result.AssistantMessage != nil {
			pieces = append(pieces, r.Result.AssistantMessage.JoinedText())
		}
	}
	got := strings.Join(pieces, "")
	if !strings.Contains(got, "hello world") {
		t.Errorf("accumulated text = %q; want to contain 'hello world'", got)
	}

	last := resps[len(resps)-1]
	if last.Metadata.Usage == nil || last.Metadata.Usage.PromptTokens != 3 {
		t.Errorf("last chunk usage = %+v; want PromptTokens=3", last.Metadata.Usage)
	}
}

func TestChatModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newChatModel(t, srv.URL, "gpt-4o")
	if m.Metadata().Provider != openai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, openai.Provider)
	}
}

func TestChatModel_Call_ErrorPropagation(t *testing.T) {
	srv := testutil.JSONServer(http.StatusUnauthorized, `{"error":{"message":"bad key"}}`)
	t.Cleanup(srv.Close)
	m := newChatModel(t, srv.URL, "gpt-4o")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if _, err := m.Call(context.Background(), req); err == nil {
		t.Fatal("expected error on 401")
	}
}
