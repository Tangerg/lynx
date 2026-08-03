package perplexity

import (
	"encoding/json"
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
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body struct {
			Query   string   `json:"query"`
			Results int      `json:"max_results"`
			Domains []string `json:"search_domain_filter"`
			Recency string   `json:"search_recency_filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body.Query != "lynx" || body.Results != 20 || body.Recency != "day" || len(body.Domains) != 1 || body.Domains[0] != "-blocked.example" {
			t.Errorf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Lynx","url":"https://example.com","snippet":"cat","date":"2026-08-03"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(t.Context(), &websearch.Request{Query: "lynx", MaxResults: 99, BlockedDomains: []string{"blocked.example"}, Recency: websearch.RecencyDay})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].PublishedTime.IsZero() {
		t.Fatalf("response = %#v", response)
	}
}
