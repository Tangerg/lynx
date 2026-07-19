package httpreq

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewClient_RequiresAllowlist(t *testing.T) {
	if _, err := NewClient(Config{}); !errors.Is(err, ErrMissingHosts) {
		t.Fatalf("want ErrMissingHosts, got %v", err)
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

	client, err := NewClient(Config{AllowedHosts: []string{hostOnly}})
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

func TestDo_RedirectHostAllowlist(t *testing.T) {
	t.Run("allows redirect to permitted host", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "redirected")
		}))
		t.Cleanup(target.Close)
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
		t.Cleanup(source.Close)

		client, err := NewClient(Config{AllowedHosts: []string{testURLHostname(t, source.URL)}})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(t.Context(), &Request{URL: source.URL})
		if err != nil {
			t.Fatalf("follow permitted redirect: %v", err)
		}
		if resp.Body != "redirected" {
			t.Fatalf("body = %q, want redirected", resp.Body)
		}
	})

	t.Run("rejects redirect to blocked host", func(t *testing.T) {
		var targetHit atomic.Bool
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			targetHit.Store(true)
			_, _ = io.WriteString(w, "secret")
		}))
		t.Cleanup(target.Close)
		blockedTarget := testURLWithHostname(t, target.URL, "localhost")
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, blockedTarget, http.StatusFound)
		}))
		t.Cleanup(source.Close)

		callerPolicyErr := errors.New("caller redirect policy")
		httpClient := &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return callerPolicyErr
			},
		}
		client, err := NewClient(Config{
			AllowedHosts: []string{testURLHostname(t, source.URL)},
			HTTPClient:   httpClient,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := httpClient.CheckRedirect(nil, nil); !errors.Is(err, callerPolicyErr) {
			t.Fatalf("NewClient mutated caller-owned redirect policy: %v", err)
		}
		_, err = client.Do(t.Context(), &Request{URL: source.URL})
		if !errors.Is(err, ErrHostNotAllowed) {
			t.Fatalf("redirect error = %v, want ErrHostNotAllowed", err)
		}
		if targetHit.Load() {
			t.Fatal("blocked redirect reached its target")
		}
	})
}

func TestDo_WildcardHost(t *testing.T) {
	client, err := NewClient(Config{AllowedHosts: []string{"*.example.com"}})
	if err != nil {
		t.Fatal(err)
	}

	if !client.allowedHosts.Allows("api.example.com") {
		t.Error("api.example.com should match *.example.com")
	}
	if !client.allowedHosts.Allows("a.b.example.com") {
		t.Error("a.b.example.com should match *.example.com")
	}
	if client.allowedHosts.Allows("example.com") {
		t.Error("example.com should NOT match *.example.com (suffix-only)")
	}
	if client.allowedHosts.Allows("evilexample.com") {
		t.Error("evilexample.com should NOT match *.example.com")
	}
}

func TestDo_MethodAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Method))
	}))
	t.Cleanup(srv.Close)

	hostOnly := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]

	client, err := NewClient(Config{AllowedHosts: []string{hostOnly}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Do(t.Context(), &Request{URL: srv.URL, Method: "POST"})
	if !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("default methods should block POST, got %v", err)
	}

	writeClient, err := NewClient(Config{
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

	client, err := NewClient(Config{
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
	client, err := NewClient(Config{AllowedHosts: []string{"example.com"}})
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
	client, err := NewClient(Config{
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

func testURLHostname(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	return parsed.Hostname()
}

func testURLWithHostname(t *testing.T, rawURL, hostname string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	parsed.Host = net.JoinHostPort(hostname, parsed.Port())
	return parsed.String()
}
