package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/oauth2"

	"github.com/Tangerg/lynx/app/runtime/internal/component/httporigin"
)

// OAuthSessionStore is the durable boundary for MCP OAuth credentials. The
// MCP infrastructure owns the opaque payload; storage implementations only
// bind it to one configured server and normalized HTTP origin.
type OAuthSessionStore interface {
	LoadOAuthSession(ctx context.Context, server, origin string) (payload []byte, found bool, err error)
	SaveOAuthSession(ctx context.Context, server, origin string, payload []byte) error
	RemoveOAuthSession(ctx context.Context, server string) error
}

const oauthSessionVersion = 1

var errStoredOAuthRejected = errors.New("mcp oauth: stored session was rejected; sign in again")

type storedOAuthSession struct {
	Version int               `json:"version"`
	Config  storedOAuthConfig `json:"config"`
	Token   storedOAuthToken  `json:"token"`
}

type storedOAuthConfig struct {
	ClientID     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret,omitempty"`
	AuthURL      string           `json:"authUrl"`
	TokenURL     string           `json:"tokenUrl"`
	AuthStyle    oauth2.AuthStyle `json:"authStyle,omitempty"`
	RedirectURL  string           `json:"redirectUrl"`
	Scopes       []string         `json:"scopes,omitempty"`
}

type storedOAuthToken struct {
	AccessToken  string    `json:"accessToken"`
	TokenType    string    `json:"tokenType,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

func oauthOrigin(endpoint string) (string, error) {
	origin, err := httporigin.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("mcp oauth: invalid endpoint origin: %w", err)
	}
	return origin.String(), nil
}

func encodeOAuthSession(cfg *oauth2.Config, token *oauth2.Token) ([]byte, error) {
	if cfg == nil || token == nil {
		return nil, errors.New("mcp oauth: config and token are required")
	}
	session := storedOAuthSession{
		Version: oauthSessionVersion,
		Config: storedOAuthConfig{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			AuthURL:      cfg.Endpoint.AuthURL,
			TokenURL:     cfg.Endpoint.TokenURL,
			AuthStyle:    cfg.Endpoint.AuthStyle,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       slices.Clone(cfg.Scopes),
		},
		Token: storedOAuthToken{
			AccessToken:  token.AccessToken,
			TokenType:    token.TokenType,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		},
	}
	if err := session.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: encode session: %w", err)
	}
	return payload, nil
}

func decodeOAuthSession(payload []byte) (*oauth2.Config, *oauth2.Token, error) {
	var session storedOAuthSession
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&session); err != nil {
		return nil, nil, fmt.Errorf("mcp oauth: decode session: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("mcp oauth: decode session: trailing data")
	}
	if err := session.validate(); err != nil {
		return nil, nil, err
	}
	return &oauth2.Config{
			ClientID:     session.Config.ClientID,
			ClientSecret: session.Config.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:   session.Config.AuthURL,
				TokenURL:  session.Config.TokenURL,
				AuthStyle: session.Config.AuthStyle,
			},
			RedirectURL: session.Config.RedirectURL,
			Scopes:      slices.Clone(session.Config.Scopes),
		}, &oauth2.Token{
			AccessToken:  session.Token.AccessToken,
			TokenType:    session.Token.TokenType,
			RefreshToken: session.Token.RefreshToken,
			Expiry:       session.Token.Expiry,
		}, nil
}

func (session storedOAuthSession) validate() error {
	if session.Version != oauthSessionVersion {
		return fmt.Errorf("mcp oauth: unsupported session version %d", session.Version)
	}
	if session.Config.ClientID == "" {
		return errors.New("mcp oauth: stored client id is empty")
	}
	for field, raw := range map[string]string{
		"authorization URL": session.Config.AuthURL,
		"token URL":         session.Config.TokenURL,
		"redirect URL":      session.Config.RedirectURL,
	} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("mcp oauth: stored %s is not an absolute HTTP(S) URL", field)
		}
	}
	if session.Token.AccessToken == "" {
		return errors.New("mcp oauth: stored access token is empty")
	}
	return nil
}

func persistOAuthSession(ctx context.Context, store OAuthSessionStore, server, origin string, cfg *oauth2.Config, token *oauth2.Token) error {
	payload, err := encodeOAuthSession(cfg, token)
	if err != nil {
		return err
	}
	if err := store.SaveOAuthSession(ctx, server, origin, payload); err != nil {
		return fmt.Errorf("mcp oauth: persist session for %q: %w", server, err)
	}
	return nil
}

type savingTokenSource struct {
	mu     sync.Mutex
	source oauth2.TokenSource
	config oauth2.Config
	last   *oauth2.Token
	save   func(*oauth2.Config, *oauth2.Token) error
}

type invalidatingTokenSource struct {
	mu          sync.Mutex
	source      oauth2.TokenSource
	store       OAuthSessionStore
	server      string
	invalidated bool
}

func newSavingTokenSource(source oauth2.TokenSource, cfg *oauth2.Config, token *oauth2.Token, save func(*oauth2.Config, *oauth2.Token) error) oauth2.TokenSource {
	config := *cfg
	config.Scopes = slices.Clone(cfg.Scopes)
	return &savingTokenSource{source: source, config: config, last: cloneOAuthToken(token), save: save}
}

func (source *savingTokenSource) Token() (*oauth2.Token, error) {
	source.mu.Lock()
	defer source.mu.Unlock()

	token, err := source.source.Token()
	if err != nil {
		return nil, err
	}
	if sameOAuthToken(source.last, token) {
		return token, nil
	}
	if err := source.save(&source.config, token); err != nil {
		return nil, err
	}
	source.last = cloneOAuthToken(token)
	return token, nil
}

func cloneOAuthToken(token *oauth2.Token) *oauth2.Token {
	if token == nil {
		return nil
	}
	clone := *token
	return &clone
}

func sameOAuthToken(left, right *oauth2.Token) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.AccessToken == right.AccessToken &&
		left.TokenType == right.TokenType &&
		left.RefreshToken == right.RefreshToken &&
		left.Expiry.Equal(right.Expiry)
}

func (source *invalidatingTokenSource) Token() (*oauth2.Token, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.invalidated {
		return nil, &dialFailure{kind: dialFailureNeedsAuth, err: errStoredOAuthRejected}
	}
	token, err := source.source.Token()
	if err == nil || !oauthCredentialRejected(err) {
		return token, err
	}
	source.invalidated = true
	removeErr := source.store.RemoveOAuthSession(context.Background(), source.server)
	return nil, &dialFailure{
		kind: dialFailureNeedsAuth,
		err:  errors.Join(errStoredOAuthRejected, err, removeErr),
	}
}

func oauthCredentialRejected(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return false
	}
	switch retrieveErr.ErrorCode {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return true
	}
	return retrieveErr.Response != nil &&
		(retrieveErr.Response.StatusCode == http.StatusUnauthorized ||
			retrieveErr.Response.StatusCode == http.StatusForbidden)
}

func invalidateRejectedTokens(source oauth2.TokenSource, store OAuthSessionStore, server string) oauth2.TokenSource {
	if store == nil {
		return source
	}
	return &invalidatingTokenSource{source: source, store: store, server: server}
}

// restoredOAuthHandler deliberately does not start an interactive browser flow
// during boot. A rejected credential is removed and the dial is classified as
// needsAuth; the explicit Authorize command then owns user interaction.
type restoredOAuthHandler struct {
	mu     sync.RWMutex
	source oauth2.TokenSource
	store  OAuthSessionStore
	server string
}

var _ auth.OAuthHandler = (*restoredOAuthHandler)(nil)

func (handler *restoredOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	handler.mu.RLock()
	defer handler.mu.RUnlock()
	return handler.source, nil
}

func (handler *restoredOAuthHandler) Authorize(ctx context.Context, _ *http.Request, response *http.Response) error {
	var responseErr error
	if response != nil && response.Body != nil {
		_, drainErr := io.Copy(io.Discard, response.Body)
		responseErr = errors.Join(drainErr, response.Body.Close())
	}
	handler.mu.Lock()
	handler.source = nil
	handler.mu.Unlock()
	removeErr := handler.store.RemoveOAuthSession(ctx, handler.server)
	return errors.Join(errStoredOAuthRejected, responseErr, removeErr)
}

func restoreOAuthHandler(ctx context.Context, store OAuthSessionStore, server, endpoint string) (auth.OAuthHandler, error) {
	if store == nil {
		return nil, nil
	}
	origin, err := oauthOrigin(endpoint)
	if err != nil {
		return nil, err
	}
	payload, found, err := store.LoadOAuthSession(ctx, server, origin)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: load session for %q: %w", server, err)
	}
	if !found {
		return nil, nil
	}
	cfg, token, err := decodeOAuthSession(payload)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: restore session for %q: %w", server, err)
	}
	if token.RefreshToken == "" && !token.Valid() {
		if err := store.RemoveOAuthSession(ctx, server); err != nil {
			return nil, fmt.Errorf("mcp oauth: remove expired session for %q: %w", server, err)
		}
		return nil, nil
	}
	source := newSavingTokenSource(
		cfg.TokenSource(context.Background(), token),
		cfg,
		token,
		func(updatedConfig *oauth2.Config, updatedToken *oauth2.Token) error {
			return persistOAuthSession(context.Background(), store, server, origin, updatedConfig, updatedToken)
		},
	)
	source = invalidateRejectedTokens(source, store, server)
	return &restoredOAuthHandler{source: source, store: store, server: server}, nil
}
