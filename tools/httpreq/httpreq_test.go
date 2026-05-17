package httpreq

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_RequiresAllowlist(t *testing.T) {
	if _, err := NewClient(&Config{}); !errors.Is(err, ErrMissingHosts) {
		t.Fatalf("want ErrMissingHosts, got %v", err)
	}
	if _, err := NewClient(nil); !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("want ErrMissingConfig, got %v", err)
	}
}

func TestDo_HostAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	srvURL := srv.URL
	host := strings.TrimPrefix(strings.TrimPrefix(srvURL, "http://"), "https://")
	hostOnly := strings.Split(host, ":")[0]

	client, err := NewClient(&Config{AllowedHosts: []string{hostOnly}})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(t.Context(), &Request{URL: srvURL + "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || resp.Body != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	if _, err := client.Do(t.Context(), &Request{URL: "https://blocked.example.com/x"}); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("want ErrHostNotAllowed, got %v", err)
	}
}

func TestDo_WildcardHost(t *testing.T) {
	client, err := NewClient(&Config{AllowedHosts: []string{"*.example.com"}})
	if err != nil {
		t.Fatal(err)
	}

	if !client.hostAllowed("api.example.com") {
		t.Error("api.example.com should match *.example.com")
	}
	if !client.hostAllowed("a.b.example.com") {
		t.Error("a.b.example.com should match *.example.com")
	}
	if client.hostAllowed("example.com") {
		t.Error("example.com should NOT match *.example.com (suffix-only)")
	}
	if client.hostAllowed("evilexample.com") {
		t.Error("evilexample.com should NOT match *.example.com")
	}
}

func TestDo_MethodAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Method))
	}))
	t.Cleanup(srv.Close)

	hostOnly := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	client, err := NewClient(&Config{AllowedHosts: []string{hostOnly}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Do(t.Context(), &Request{URL: srv.URL, Method: "POST"}); !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("default methods should block POST, got %v", err)
	}

	writeClient, err := NewClient(&Config{
		AllowedHosts:   []string{hostOnly},
		AllowedMethods: []string{"GET", "POST"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := writeClient.Do(t.Context(), &Request{URL: srv.URL, Method: "post"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body != "POST" {
		t.Fatalf("server saw method %q, want POST", resp.Body)
	}
}

func TestDo_ResponseTruncation(t *testing.T) {
	payload := strings.Repeat("x", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)

	hostOnly := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	client, err := NewClient(&Config{
		AllowedHosts:     []string{hostOnly},
		MaxResponseBytes: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(t.Context(), &Request{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated {
		t.Error("expected Truncated=true")
	}
	if len(resp.Body) != 100 {
		t.Errorf("body length = %d, want 100", len(resp.Body))
	}
}

func TestDo_InvalidURL(t *testing.T) {
	client, err := NewClient(&Config{AllowedHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "ftp://example.com", "not-a-url", "/relative"} {
		if _, err := client.Do(t.Context(), &Request{URL: bad}); err == nil {
			t.Errorf("URL %q should be rejected", bad)
		}
	}
}

func TestDo_DefaultHeadersAndQuery(t *testing.T) {
	var sawAuth, sawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawQuery = r.URL.Query().Get("q")
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	hostOnly := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]
	client, err := NewClient(&Config{
		AllowedHosts:   []string{hostOnly},
		DefaultHeaders: map[string]string{"Authorization": "Bearer secret"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Do(t.Context(), &Request{
		URL:   srv.URL,
		Query: map[string]string{"q": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer secret" {
		t.Errorf("DefaultHeaders not applied; got %q", sawAuth)
	}
	if sawQuery != "hello" {
		t.Errorf("Query not applied; got %q", sawQuery)
	}
}
