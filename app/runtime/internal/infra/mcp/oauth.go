package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// OAuth for remote (HTTP) MCP servers — in-memory, per session. A server that
// needs OAuth surfaces as needsAuth (a 401 on dial maps there via dialStatus),
// which is the cue to sign in. [Connections.Authorize] then runs the
// authorization-code flow: open the system browser to the authorization URL,
// catch the redirect on a loopback callback server, exchange the code — all
// driven by the go-sdk's AuthorizationCodeHandler (discovery + Dynamic Client
// Registration + PKCE + refresh). The resulting handler is held on the live
// server for the process lifetime (reused on reconnect, auto-refreshing within
// the session); it is NOT persisted, so a restart re-prompts. This matches
// Continue's model — the SDK exposes no token-seed hook to rebuild a refreshing
// source from a stored token, so durable sign-in would need a custom
// discovery+DCR layer (a deliberate follow-up, not done here).

// oauthCallbackPath is the loopback redirect path the authorization server sends
// the code back to.
const oauthCallbackPath = "/callback"

// oauthFlowTimeout bounds one interactive sign-in — discovery plus the human
// completing the browser flow. Generous, because a person is in the loop.
const oauthFlowTimeout = 5 * time.Minute

// oauthCallback is the authorization-server redirect outcome.
type oauthCallback struct {
	code  string
	state string
	err   error
}

// oauthFlow is one interactive authorization: a loopback HTTP server on an
// ephemeral port catching the redirect, plus the browser-open + wait the
// go-sdk's [auth.AuthorizationCodeFetcher] drives. The redirect URI (known only
// after binding) is registered via Dynamic Client Registration.
type oauthFlow struct {
	redirectURI string
	server      *http.Server
	result      chan oauthCallback
}

// newOAuthFlow binds a loopback callback server on an ephemeral port and starts
// serving. The caller closes it once the flow settles.
func newOAuthFlow() (*oauthFlow, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: bind callback server: %w", err)
	}
	f := &oauthFlow{
		redirectURI: fmt.Sprintf("http://%s%s", ln.Addr().String(), oauthCallbackPath),
		result:      make(chan oauthCallback, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(oauthCallbackPath, f.handleCallback)
	f.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = f.server.Serve(ln) }() // returns on Shutdown
	return f, nil
}

// handleCallback captures the authorization code (or error) from the redirect,
// shows the user a close-this-tab page, and hands the outcome to fetch.
func (f *oauthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cb := oauthCallback{code: q.Get("code"), state: q.Get("state")}
	switch {
	case q.Get("error") != "":
		cb.err = fmt.Errorf("authorization denied: %s", q.Get("error"))
	case cb.code == "":
		cb.err = errors.New("authorization response missing code")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if cb.err != nil {
		_, _ = w.Write([]byte(oauthResultHTML("Authorization failed — you can close this tab.")))
	} else {
		_, _ = w.Write([]byte(oauthResultHTML("Authorized — close this tab and return to Lyra.")))
	}
	select {
	case f.result <- cb:
	default:
	}
}

// fetch is the [auth.AuthorizationCodeFetcher]: open the browser to the
// authorization URL, then wait for the loopback redirect (or ctx timeout).
func (f *oauthFlow) fetch(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	if err := openBrowser(args.URL); err != nil {
		return nil, fmt.Errorf("mcp oauth: open browser (visit %s manually): %w", args.URL, err)
	}
	select {
	case cb := <-f.result:
		if cb.err != nil {
			return nil, cb.err
		}
		return &auth.AuthorizationResult{Code: cb.code, State: cb.state}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *oauthFlow) close() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), time.Second)
	defer cancel()
	_ = f.server.Shutdown(ctx)
}

// newOAuthHandler builds the interactive authorization-code handler for one
// sign-in, registering the flow's loopback redirect via Dynamic Client
// Registration so no client id need be preconfigured.
func newOAuthHandler(flow *oauthFlow) (auth.OAuthHandler, error) {
	return auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:              "Lyra",
				RedirectURIs:            []string{flow.redirectURI},
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
				TokenEndpointAuthMethod: "none",
			},
		},
		RedirectURL:              flow.redirectURI,
		AuthorizationCodeFetcher: flow.fetch,
	})
}

// openBrowser opens url in the user's default browser, best-effort and
// platform-specific. The runtime is local (the desktop app's loopback), so this
// opens on the user's machine; an error is surfaced so the URL can be opened
// manually.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func oauthResultHTML(msg string) string {
	return "<!doctype html><meta charset=utf-8><title>Lyra</title>" +
		`<body style="font:14px system-ui;display:grid;place-items:center;height:100vh;margin:0">` +
		"<p>" + msg + "</p></body>"
}
