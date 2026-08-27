package httpreq

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolUsesStrictTypedContract(t *testing.T) {
	client, err := NewClient(Config{AllowedHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := NewTool(client)
	if err != nil {
		t.Fatal(err)
	}
	definition := tool.Definition()
	if definition.Name != "http_request" {
		t.Fatalf("name = %q, want http_request", definition.Name)
	}
	var schema struct {
		AdditionalProperties bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema.AdditionalProperties {
		t.Fatalf("schema permits unknown fields: %s", definition.InputSchema)
	}
	for _, arguments := range []string{
		`{"url":"https://example.com","timeout":5000}`,
		`{"url":"https://example.com","method":"get"}`,
		`{"url":"https://example.com","timeout_ms":120001}`,
	} {
		if _, err := tool.Call(t.Context(), arguments); err == nil {
			t.Errorf("Call(%s): want contract error", arguments)
		}
	}
}

func TestNewClient_RequiresAllowlist(t *testing.T) {
	if _, err := NewClient(Config{}); !errors.Is(err, ErrMissingHosts) {
		t.Fatalf("want ErrMissingHosts, got %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   error
	}{
		{name: "valid", config: Config{AllowedHosts: []string{"example.com"}}},
		{name: "missing hosts", config: Config{}, want: ErrMissingHosts},
		{name: "invalid host", config: Config{AllowedHosts: []string{"bad*host"}}, want: ErrInvalidConfig},
		{name: "blank method", config: Config{AllowedHosts: []string{"example.com"}, AllowedMethods: []Method{""}}, want: ErrInvalidConfig},
		{name: "unsupported method", config: Config{AllowedHosts: []string{"example.com"}, AllowedMethods: []Method{"CONNECT"}}, want: ErrInvalidMethod},
		{name: "negative timeout", config: Config{AllowedHosts: []string{"example.com"}, DefaultTimeout: -time.Second}, want: ErrInvalidConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.want == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMethod(t *testing.T) {
	if got := Method("").Resolve(); got != MethodGET {
		t.Fatalf("empty Method.Resolve() = %q, want %q", got, MethodGET)
	}
	if got := Method(" post ").Resolve(); got != MethodPOST {
		t.Fatalf("Method(post).Resolve() = %q, want %q", got, MethodPOST)
	}
	if err := Method("CONNECT").Validate(); !errors.Is(err, ErrInvalidMethod) {
		t.Fatalf("Method(CONNECT).Validate() error = %v, want ErrInvalidMethod", err)
	}
}

func TestRequestPrepareReturnsOwnedNormalizedCopy(t *testing.T) {
	original := &Request{
		URL:     "  https://example.com/path  ",
		Method:  " post ",
		Headers: map[string]string{"X-Test": "original"},
		Query:   map[string]string{"q": "scope"},
	}
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
	prepared, err := original.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	prepared.Headers["X-Test"] = "changed"
	prepared.Query["q"] = "changed"

	if prepared.URL != "https://example.com/path" || prepared.Method != MethodPOST {
		t.Fatalf("prepared request = %#v", prepared)
	}
	if original.URL != "  https://example.com/path  " || original.Method != " post " ||
		original.Headers["X-Test"] != "original" || original.Query["q"] != "scope" {
		t.Fatalf("Prepare mutated its input: %#v", original)
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
		if checkRedirectErr := httpClient.CheckRedirect(nil, nil); !errors.Is(checkRedirectErr, callerPolicyErr) {
			t.Fatalf("NewClient mutated caller-owned redirect policy: %v", checkRedirectErr)
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
		AllowedMethods: []Method{MethodGET, MethodPOST},
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

func TestDo_ValidatesMethodAndTimeout(t *testing.T) {
	client, err := NewClient(Config{AllowedHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		request Request
		want    error
	}{
		{name: "method", request: Request{URL: "https://example.com", Method: "CONNECT"}, want: ErrInvalidMethod},
		{name: "negative timeout", request: Request{URL: "https://example.com", TimeoutMS: -1}, want: ErrInvalidTimeout},
		{name: "large timeout", request: Request{URL: "https://example.com", TimeoutMS: maxRequestTimeout + 1}, want: ErrInvalidTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.Do(t.Context(), &test.request); !errors.Is(err, test.want) {
				t.Fatalf("Do() error = %v, want %v", err, test.want)
			}
		})
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
