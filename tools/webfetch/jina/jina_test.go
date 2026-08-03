package jina

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/tools/webfetch"
)

func TestProvider(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("NewClient accepted an empty API key")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Return-Format"); got != "text" {
			t.Errorf("X-Return-Format = %q", got)
		}
		if got := r.Header.Get("X-Retain-Images"); got != "none" {
			t.Errorf("X-Retain-Images = %q", got)
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body.URL != "https://example.com" {
			t.Errorf("url = %q", body.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"content":"example"}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Fetch(t.Context(), &webfetch.Request{URL: "https://example.com", Format: webfetch.FormatText})
	if err != nil {
		t.Fatal(err)
	}
	if response.Format != webfetch.FormatText || response.Content != "example" {
		t.Fatalf("response = %#v", response)
	}
}
