package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/oauth2"
)

type memoryOAuthStore struct {
	mu      sync.Mutex
	origin  string
	payload []byte
	removed int
	saveErr error
}

func (m *memoryOAuthStore) LoadOAuthSession(_ context.Context, _ string, origin string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.origin != origin || len(m.payload) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), m.payload...), true, nil
}

func (m *memoryOAuthStore) SaveOAuthSession(_ context.Context, _ string, origin string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.origin = origin
	m.payload = append([]byte(nil), payload...)
	return nil
}

func (m *memoryOAuthStore) RemoveOAuthSession(context.Context, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payload = nil
	m.removed++
	return nil
}

type tokenSourceFunc func() (*oauth2.Token, error)

func (t tokenSourceFunc) Token() (*oauth2.Token, error) { return t() }

func oauthSessionFixture(t *testing.T, token *oauth2.Token) (*oauth2.Config, []byte) {
	t.Helper()
	cfg := &oauth2.Config{
		ClientID: "client-id",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://auth.example/authorize",
			TokenURL: "https://auth.example/token",
		},
		RedirectURL: "http://127.0.0.1:3000/callback",
		Scopes:      []string{"tools.read"},
	}
	payload, err := encodeOAuthSession(cfg, token)
	if err != nil {
		t.Fatalf("encodeOAuthSession: %v", err)
	}
	return cfg, payload
}

func TestOAuthSessionRoundTripOwnsSlices(t *testing.T) {
	token := &oauth2.Token{
		AccessToken: "access", TokenType: "Bearer", RefreshToken: "refresh",
		Expiry: time.Now().Add(time.Hour).Round(time.Second),
	}
	cfg, payload := oauthSessionFixture(t, token)
	cfg.Scopes[0] = "mutated"

	gotConfig, gotToken, err := decodeOAuthSession(payload)
	if err != nil {
		t.Fatalf("decodeOAuthSession: %v", err)
	}
	if gotConfig.Scopes[0] != "tools.read" || gotToken.AccessToken != token.AccessToken ||
		gotToken.RefreshToken != token.RefreshToken || !gotToken.Expiry.Equal(token.Expiry) {
		t.Fatalf("decoded session = config %+v token %+v", gotConfig, gotToken)
	}
}

func TestSavingTokenSourcePersistsOnlyChangedToken(t *testing.T) {
	initial := &oauth2.Token{AccessToken: "initial", Expiry: time.Now().Add(time.Minute)}
	refreshed := &oauth2.Token{AccessToken: "refreshed", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}
	cfg, _ := oauthSessionFixture(t, initial)
	var calls int
	source := newSavingTokenSource(tokenSourceFunc(func() (*oauth2.Token, error) {
		return refreshed, nil
	}), cfg, initial, func(*oauth2.Config, *oauth2.Token) error {
		calls++
		return nil
	})

	for range 2 {
		if token, err := source.Token(); err != nil || token.AccessToken != refreshed.AccessToken {
			t.Fatalf("Token = %+v, %v", token, err)
		}
	}
	if calls != 1 {
		t.Fatalf("save calls = %d, want 1", calls)
	}
}

func TestSavingTokenSourceFailsClosedWhenRefreshCannotPersist(t *testing.T) {
	initial := &oauth2.Token{AccessToken: "initial", Expiry: time.Now().Add(time.Minute)}
	refreshed := &oauth2.Token{AccessToken: "refreshed", Expiry: time.Now().Add(time.Hour)}
	cfg, _ := oauthSessionFixture(t, initial)
	wantErr := errors.New("disk unavailable")
	source := newSavingTokenSource(tokenSourceFunc(func() (*oauth2.Token, error) {
		return refreshed, nil
	}), cfg, initial, func(*oauth2.Config, *oauth2.Token) error { return wantErr })

	if token, err := source.Token(); token != nil || !errors.Is(err, wantErr) {
		t.Fatalf("Token = %+v, %v, want persistence error", token, err)
	}
}

func TestInvalidatingTokenSourceDeletesRejectedRefresh(t *testing.T) {
	store := &memoryOAuthStore{payload: []byte("saved")}
	source := invalidateRejectedTokens(tokenSourceFunc(func() (*oauth2.Token, error) {
		return nil, &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
	}), t.Context(), store, "remote")

	if token, err := source.Token(); token != nil || dialStatus(err) != "needsAuth" {
		t.Fatalf("Token = %+v, %v, want needsAuth", token, err)
	}
	if store.removed != 1 || len(store.payload) != 0 {
		t.Fatalf("rejected refresh was not removed: %+v", store)
	}
	if _, err := source.Token(); dialStatus(err) != "needsAuth" || store.removed != 1 {
		t.Fatalf("repeated Token = %v, removed=%d", err, store.removed)
	}
}

func TestRestoreOAuthHandlerRejectsCredentialWithoutInteractiveFlow(t *testing.T) {
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}
	_, payload := oauthSessionFixture(t, token)
	store := &memoryOAuthStore{origin: "https://mcp.example:443", payload: payload}

	handler, err := restoreOAuthHandler(t.Context(), t.Context(), store, "remote", "https://MCP.example/tools")
	if err != nil {
		t.Fatalf("restoreOAuthHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("restoreOAuthHandler returned nil")
	}
	source, err := handler.TokenSource(t.Context())
	if err != nil {
		t.Fatalf("TokenSource: %v", err)
	}
	if got, tokenErr := source.Token(); tokenErr != nil || got.AccessToken != token.AccessToken {
		t.Fatalf("restored token = %+v, %v", got, tokenErr)
	}

	response := &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}
	err = handler.Authorize(t.Context(), nil, response)
	if !errors.Is(err, errStoredOAuthRejected) {
		t.Fatalf("Authorize error = %v", err)
	}
	if source, err := handler.TokenSource(t.Context()); err != nil || source != nil {
		t.Fatalf("TokenSource after rejection = %v, %v", source, err)
	}
	if store.removed != 1 || len(store.payload) != 0 {
		t.Fatalf("rejected session was not removed: %+v", store)
	}
}

func TestRestoreOAuthHandlerRejectsMalformedPayload(t *testing.T) {
	store := &memoryOAuthStore{
		origin:  "https://mcp.example:443",
		payload: []byte(`{"version":1,"unknown":true}`),
	}
	var handler auth.OAuthHandler
	handler, err := restoreOAuthHandler(t.Context(), t.Context(), store, "remote", "https://mcp.example/tools")
	if handler != nil || err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("restore malformed = handler %v, err %v", handler, err)
	}
}
