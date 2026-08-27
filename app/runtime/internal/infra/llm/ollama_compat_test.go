package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

func TestOllamaOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "default", want: defaultOllamaOpenAIBaseURL},
		{name: "daemon root", configured: "http://host:11434", want: "http://host:11434/v1"},
		{name: "trailing slash", configured: "http://host:11434/", want: "http://host:11434/v1"},
		{name: "already compatible", configured: "http://host:11434/v1/", want: "http://host:11434/v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ollamaOpenAIBaseURL(test.configured); got != test.want {
				t.Fatalf("ollamaOpenAIBaseURL(%q) = %q, want %q", test.configured, got, test.want)
			}
		})
	}
}

func TestOllamaCompatibleChatUsesProviderScopedV1Protocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "Bearer ollama" {
			t.Errorf("Authorization = %q, want fallback Ollama credential", authorization)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		if body.Model != "qwen3:8b" {
			t.Errorf("model = %q, want qwen3:8b", body.Model)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chatcmpl-1","model":"qwen3:8b","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(server.Close)

	model, err := buildOllamaChatModel(
		ClientSpec{Provider: ProviderOllama, Model: "qwen3:8b", BaseURL: server.URL},
		chat.Options{Model: "qwen3:8b"},
	)
	if err != nil {
		t.Fatalf("buildOllamaChatModel: %v", err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := model.Call(t.Context(), request)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := response.Text(); got != "ok" {
		t.Fatalf("response text = %q, want ok", got)
	}
	if _, ok := response.Metadata.Extra["ollama/openai_response"]; !ok {
		t.Fatal("response did not retain provider-scoped ollama/openai_response")
	}
}

func TestOllamaCompatibleEmbeddingUsesV1Protocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %q, want /v1/embeddings", request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "Bearer configured-key" {
			t.Errorf("Authorization = %q, want configured credential", authorization)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"object":"list","model":"nomic-embed-text","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)

	opts, err := embedding.NewOptions("nomic-embed-text")
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	model, err := buildOllamaEmbeddingModel(
		ClientSpec{Provider: ProviderOllama, Model: "nomic-embed-text", APIKey: "configured-key", BaseURL: server.URL + "/v1/"},
		opts,
	)
	if err != nil {
		t.Fatalf("buildOllamaEmbeddingModel: %v", err)
	}
	request, err := embedding.NewRequest([]string{"hello"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := model.Call(t.Context(), request)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(response.Outputs) != 1 || len(response.Outputs[0].Embedding) != 2 {
		t.Fatalf("embedding results = %#v", response.Outputs)
	}
	if response.Metadata == nil || response.Metadata.Model != "nomic-embed-text" {
		t.Fatalf("embedding metadata = %#v", response.Metadata)
	}
}
