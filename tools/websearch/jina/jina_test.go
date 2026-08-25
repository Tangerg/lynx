package jina

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/tools/websearch"
)

func TestProvider(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("NewClient accepted an empty API key")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/lynx agent" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Respond-With"); got != "no-content" {
			t.Errorf("X-Respond-With = %q", got)
		}
		if got := r.URL.Query().Get("count"); got != "20" {
			t.Errorf("count = %q", got)
		}
		if got := r.URL.Query().Get("site"); got != "example.com" {
			t.Errorf("site = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"title":"Lynx","url":"https://example.com","description":"cat","date":"2026-08-03"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(t.Context(), &websearch.Request{Query: "lynx agent", MaxResults: 20, AllowedDomains: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Snippet != "cat" {
		t.Fatalf("response = %#v", response)
	}
}
