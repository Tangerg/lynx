package tavily

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
			Query      string   `json:"query"`
			Results    int      `json:"max_results"`
			TimeRange  string   `json:"time_range"`
			Domains    []string `json:"include_domains"`
			Depth      string   `json:"search_depth"`
			HasFavicon bool     `json:"include_favicon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body.Query != "lynx" || body.Results != 20 || body.TimeRange != "year" || len(body.Domains) != 1 || body.Depth != "basic" || !body.HasFavicon {
			t.Errorf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"lynx","results":[{"title":"Lynx","url":"https://example.com","content":"cat","favicon":"https://example.com/favicon.ico"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(t.Context(), &websearch.Request{Query: "lynx", MaxResults: 99, AllowedDomains: []string{"example.com"}, Recency: websearch.RecencyYear})
	if err != nil {
		t.Fatal(err)
	}
	if response.Query != "lynx" || len(response.Results) != 1 || response.Results[0].FaviconURL == "" {
		t.Fatalf("response = %#v", response)
	}
}
