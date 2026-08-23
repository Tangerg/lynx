package mcpflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	oauthCallbackPath   = "/callback"
	oauthFlowTimeout    = 5 * time.Minute
	oauthSessionVersion = 1
)

var errOAuthRequired = errors.New("mcpflow: interactive authorization required")

type oauthSession struct {
	Version int         `json:"version"`
	Config  oauthConfig `json:"config"`
	Token   oauthToken  `json:"token"`
}

type oauthConfig struct {
	ClientID     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret,omitempty"`
	AuthURL      string           `json:"authUrl"`
	TokenURL     string           `json:"tokenUrl"`
	AuthStyle    oauth2.AuthStyle `json:"authStyle,omitempty"`
	RedirectURL  string           `json:"redirectUrl"`
	Scopes       []string         `json:"scopes,omitempty"`
}

type oauthToken struct {
	AccessToken  string    `json:"accessToken"`
	TokenType    string    `json:"tokenType,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`
}

func encodeOAuthSession(config *oauth2.Config, token *oauth2.Token) ([]byte, error) {
	if config == nil || token == nil {
		return nil, errors.New("mcpflow: OAuth config and token are required")
	}
	value := oauthSession{
		Version: oauthSessionVersion,
		Config: oauthConfig{
			ClientID: config.ClientID, ClientSecret: config.ClientSecret,
			AuthURL: config.Endpoint.AuthURL, TokenURL: config.Endpoint.TokenURL,
			AuthStyle: config.Endpoint.AuthStyle, RedirectURL: config.RedirectURL,
			Scopes: slices.Clone(config.Scopes),
		},
		Token: oauthToken{
			AccessToken: token.AccessToken, TokenType: token.TokenType,
			RefreshToken: token.RefreshToken, Expiry: token.Expiry,
		},
	}
	if err := value.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func decodeOAuthSession(payload []byte) (*oauth2.Config, *oauth2.Token, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var value oauthSession
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, fmt.Errorf("mcpflow: decode OAuth session: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("mcpflow: OAuth session contains trailing data")
	}
	if err := value.validate(); err != nil {
		return nil, nil, err
	}
	return &oauth2.Config{
		ClientID: value.Config.ClientID,
		ClientSecret: value.Config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL: value.Config.AuthURL,
			TokenURL: value.Config.TokenURL,
			AuthStyle: value.Config.AuthStyle,
		},
		RedirectURL: value.Config.RedirectURL,
		Scopes: slices.Clone(value.Config.Scopes),
	}, &oauth2.Token{
		AccessToken: value.Token.AccessToken,
		TokenType: value.Token.TokenType,
		RefreshToken: value.Token.RefreshToken,
		Expiry: value.Token.Expiry,
	}, nil
}

func (value oauthSession) validate() error {
	if value.Version != oauthSessionVersion || value.Config.ClientID == "" || value.Token.AccessToken == "" {
		return errors.New("mcpflow: invalid OAuth session identity")
	}
	for label, raw := range map[string]string{
		"authorization": value.Config.AuthURL,
		"token": value.Config.TokenURL,
		"redirect": value.Config.RedirectURL,
	} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("mcpflow: invalid OAuth %s URL", label)
		}
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

func newSavingTokenSource(
	source oauth2.TokenSource,
	config *oauth2.Config,
	initial *oauth2.Token,
	save func(*oauth2.Config, *oauth2.Token) error,
) oauth2.TokenSource {
	copyConfig := *config
	copyConfig.Scopes = slices.Clone(config.Scopes)
	return &savingTokenSource{
		source: source, config: copyConfig,
		last: cloneToken(initial), save: save,
	}
}

func (source *savingTokenSource) Token() (*oauth2.Token, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	token, err := source.source.Token()
	if err != nil {
		return nil, err
	}
	if sameToken(source.last, token) {
		return token, nil
	}
	if err := source.save(&source.config, token); err != nil {
		return nil, err
	}
	source.last = cloneToken(token)
	return token, nil
}

func cloneToken(value *oauth2.Token) *oauth2.Token {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func sameToken(left, right *oauth2.Token) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.AccessToken == right.AccessToken && left.TokenType == right.TokenType &&
		left.RefreshToken == right.RefreshToken && left.Expiry.Equal(right.Expiry)
}

// oauthSessionWriter owns the exact aggregate generation that authorized a
// token write. Its local revision advances only after the SQLite CAS commits.
type oauthSessionWriter struct {
	mu            sync.Mutex
	service       *Service
	configuration mcpserver.Configuration
}

func newOAuthSessionWriter(
	service *Service,
	configuration mcpserver.Configuration,
) *oauthSessionWriter {
	return &oauthSessionWriter{service: service, configuration: configuration.Clone()}
}

func (writer *oauthSessionWriter) Save(
	ctx context.Context,
	config *oauth2.Config,
	token *oauth2.Token,
) error {
	payload, err := encodeOAuthSession(config, token)
	if err != nil {
		return err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	release, err := writer.service.lanes.Acquire(ctx, writer.configuration.Name())
	if err != nil {
		return err
	}
	defer release()
	previousRevision := writer.configuration.Revision()
	previous := writer.configuration.Clone()
	origin, err := writer.configuration.HTTPOrigin()
	if err != nil {
		return err
	}
	changed, err := writer.configuration.PutOAuth(origin, payload, writer.service.now())
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := writer.service.store.SaveMCPServer(ctx, writer.configuration, previousRevision); err != nil {
		writer.configuration = previous
		return fmt.Errorf("mcpflow: persist OAuth session: %w", err)
	}
	return nil
}

func (writer *oauthSessionWriter) Clear(ctx context.Context) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	release, err := writer.service.lanes.Acquire(ctx, writer.configuration.Name())
	if err != nil {
		return err
	}
	defer release()
	previousRevision := writer.configuration.Revision()
	previous := writer.configuration.Clone()
	if !writer.configuration.ClearOAuth(writer.service.now()) {
		return nil
	}
	if err := writer.service.store.SaveMCPServer(ctx, writer.configuration, previousRevision); err != nil {
		writer.configuration = previous
		return fmt.Errorf("mcpflow: clear OAuth session: %w", err)
	}
	return nil
}

func (writer *oauthSessionWriter) Configuration() mcpserver.Configuration {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.configuration.Clone()
}

type passiveOAuthHandler struct {
	source oauth2.TokenSource
	clear  func(context.Context) error
}

func (handler *passiveOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return handler.source, nil
}

func (handler *passiveOAuthHandler) Authorize(
	ctx context.Context,
	_ *http.Request,
	response *http.Response,
) error {
	if response != nil && response.Body != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if handler.source != nil && handler.clear != nil {
		if err := handler.clear(context.WithoutCancel(ctx)); err != nil {
			return errors.Join(errOAuthRequired, err)
		}
	}
	return errOAuthRequired
}

type oauthCallback struct {
	code   string
	state  string
	issuer string
	err    error
}

type oauthFlow struct {
	redirectURL string
	server      *http.Server
	result      chan oauthCallback
	serveDone   chan error
	openURL     func(context.Context, string) error
}

func newOAuthFlow(
	ctx context.Context,
	openURL func(context.Context, string) error,
) (*oauthFlow, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mcpflow: bind OAuth callback: %w", err)
	}
	flow := &oauthFlow{
		redirectURL: "http://" + listener.Addr().String() + oauthCallbackPath,
		result: make(chan oauthCallback, 1),
		serveDone: make(chan error, 1),
		openURL: openURL,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(oauthCallbackPath, flow.handle)
	flow.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { flow.serveDone <- flow.server.Serve(listener) }()
	return flow, nil
}

func (flow *oauthFlow) handle(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	result := oauthCallback{
		code: query.Get("code"), state: query.Get("state"), issuer: query.Get("iss"),
	}
	if providerError := query.Get("error"); providerError != "" {
		result.err = fmt.Errorf("authorization declined: %s", providerError)
	} else if result.code == "" {
		result.err = errors.New("authorization callback omitted code")
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	message := "Authorization complete. You can close this tab and return to Lyra."
	if result.err != nil {
		message = "Authorization did not complete. You can close this tab and return to Lyra."
	}
	_, _ = io.WriteString(writer,
		"<!doctype html><meta charset=utf-8><title>Lyra</title>"+
			"<body style=\"font:14px system-ui;display:grid;place-items:center;height:100vh;margin:0\">"+
			"<p>"+message+"</p></body>")
	select {
	case flow.result <- result:
	default:
	}
}

func (flow *oauthFlow) fetch(
	ctx context.Context,
	arguments *auth.AuthorizationArgs,
) (*auth.AuthorizationResult, error) {
	if err := flow.openURL(ctx, arguments.URL); err != nil {
		return nil, fmt.Errorf("mcpflow: open authorization URL: %w", err)
	}
	select {
	case result := <-flow.result:
		if result.err != nil {
			return nil, result.err
		}
		return &auth.AuthorizationResult{
			Code: result.code, State: result.state, Iss: result.issuer,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (flow *oauthFlow) close(ctx context.Context) error {
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	shutdownErr := flow.server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, flow.server.Close())
	}
	serveErr := <-flow.serveDone
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}

func openSystemBrowser(ctx context.Context, target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", target)
	case "windows":
		command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.CommandContext(ctx, "xdg-open", target)
	}
	return command.Run()
}

func oauthHTTPClient() *http.Client {
	return &http.Client{
		Transport: http.DefaultTransport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("mcpflow: too many OAuth redirects")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && request.URL.Scheme != "https" {
				return errors.New("mcpflow: OAuth redirect would downgrade HTTPS")
			}
			return nil
		},
	}
}

func newInteractiveOAuthHandler(
	flow *oauthFlow,
	writer *oauthSessionWriter,
) (auth.OAuthHandler, error) {
	configuration := writer.Configuration()
	if _, err := configuration.HTTPOrigin(); err != nil {
		return nil, err
	}
	config := &auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				RedirectURIs: []string{flow.redirectURL},
				ClientName: "Lyra",
				GrantTypes: []string{"authorization_code", "refresh_token"},
				ResponseTypes: []string{"code"},
				TokenEndpointAuthMethod: "none",
			},
		},
		RedirectURL: flow.redirectURL,
		AuthorizationCodeFetcher: flow.fetch,
		RequestRefreshToken: true,
		Client: oauthHTTPClient(),
	}
	config.NewTokenSource = func(
		ctx context.Context,
		oauthConfig *oauth2.Config,
		token *oauth2.Token,
	) (oauth2.TokenSource, error) {
		if err := writer.Save(ctx, oauthConfig, token); err != nil {
			return nil, err
		}
		return newSavingTokenSource(
			oauthConfig.TokenSource(ctx, token), oauthConfig, token,
			func(updatedConfig *oauth2.Config, updatedToken *oauth2.Token) error {
				return writer.Save(writer.service.lifetime, updatedConfig, updatedToken)
			},
		), nil
	}
	return auth.NewAuthorizationCodeHandler(config)
}

func (service *Service) passiveOAuthHandler(
	configuration mcpserver.Configuration,
) (auth.OAuthHandler, error) {
	secrets := configuration.Secrets()
	writer := newOAuthSessionWriter(service, configuration)
	handler := &passiveOAuthHandler{
		clear: func(ctx context.Context) error { return writer.Clear(ctx) },
	}
	if len(secrets.OAuthSession) == 0 {
		return handler, nil
	}
	config, token, err := decodeOAuthSession(secrets.OAuthSession)
	if err != nil {
		return nil, err
	}
	sourceContext := context.WithValue(service.lifetime, oauth2.HTTPClient, oauthHTTPClient())
	handler.source = newSavingTokenSource(
		config.TokenSource(sourceContext, token), config, token,
		func(updatedConfig *oauth2.Config, updatedToken *oauth2.Token) error {
			return writer.Save(service.lifetime, updatedConfig, updatedToken)
		},
	)
	return handler, nil
}

func (service *Service) startAuthorization(
	configuration mcpserver.Configuration,
	attempt mcpserver.AuthorizationAttempt,
) bool {
	name := configuration.Name()
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return false
	}
	live := service.live[name]
	if live == nil {
		live = &liveServer{}
		service.live[name] = live
	}
	live.generation++
	generation := live.generation
	if live.cancel != nil {
		live.cancel()
	}
	oldSession := live.session
	operationContext, cancelOperation := context.WithCancel(service.lifetime)
	live.session = nil
	live.cancel = cancelOperation
	live.tools = nil
	live.status = protocol.MCPServerState{Type: protocol.MCPServerConnecting}
	service.tasks.Add(1)
	service.mu.Unlock()
	if oldSession != nil {
		_ = oldSession.Close()
	}
	service.publish(name)
	go func() {
		defer service.tasks.Done()
		flowContext, cancelFlow := context.WithTimeout(operationContext, oauthFlowTimeout)
		session, sessionCancel, tools, err := service.dialInteractive(flowContext, configuration)
		cancelFlow()
		service.commitAuthorization(name, generation, session, sessionCancel, tools, attempt, err)
	}()
	return true
}

func (service *Service) dialInteractive(
	ctx context.Context,
	configuration mcpserver.Configuration,
) (_ *sdkmcp.ClientSession, _ context.CancelFunc, _ []protocol.MCPTool, resultErr error) {
	flow, err := newOAuthFlow(ctx, service.openURL)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, flow.close(ctx)) }()
	writer := newOAuthSessionWriter(service, configuration)
	handler, err := newInteractiveOAuthHandler(flow, writer)
	if err != nil {
		return nil, nil, nil, err
	}
	client, err := secureHTTPClient(configuration.URL(), configuration.Secrets(), false)
	if err != nil {
		return nil, nil, nil, err
	}
	sessionContext, cancelSession := context.WithCancel(service.lifetime)
	stop := context.AfterFunc(ctx, cancelSession)
	transport := &sdkmcp.StreamableClientTransport{
		Endpoint: configuration.URL(), HTTPClient: client, OAuthHandler: handler,
	}
	session, err := service.clientFor(configuration.Name(), true).Connect(sessionContext, transport, nil)
	if err != nil {
		stop()
		cancelSession()
		return nil, nil, nil, err
	}
	if !stop() || ctx.Err() != nil {
		cancelSession()
		_ = session.Close()
		return nil, nil, nil, ctx.Err()
	}
	tools, err := listTools(ctx, configuration.Name(), configuration.DisabledTools(), session)
	if err != nil {
		cancelSession()
		_ = session.Close()
		return nil, nil, nil, err
	}
	return session, cancelSession, tools, nil
}

func (service *Service) commitAuthorization(
	name string,
	generation uint64,
	session *sdkmcp.ClientSession,
	sessionCancel context.CancelFunc,
	tools []protocol.MCPTool,
	attempt mcpserver.AuthorizationAttempt,
	authorizationErr error,
) {
	service.mu.Lock()
	live := service.live[name]
	current := !service.closed && live != nil && live.generation == generation
	watch := false
	if current {
		if live.cancel != nil {
			live.cancel()
		}
		if authorizationErr == nil {
			count := len(tools)
			live.status = protocol.MCPServerState{
				Type: protocol.MCPServerConnected, ToolCount: &count,
			}
			live.session = session
			live.cancel = sessionCancel
			live.tools = tools
			service.tasks.Add(1)
			watch = true
		} else {
			live.session = nil
			live.cancel = nil
			live.tools = nil
			live.status = protocol.MCPServerState{
				Type: protocol.MCPServerNeedsAuth,
				Error: &protocol.ProblemData{Type: protocol.ProblemMCPAuthorizationFailed},
			}
		}
	}
	service.mu.Unlock()
	if !current || authorizationErr != nil {
		if sessionCancel != nil {
			sessionCancel()
		}
		if session != nil {
			_ = session.Close()
		}
	}

	status := mcpserver.AuthorizationSucceeded
	switch {
	case !current || errors.Is(authorizationErr, context.Canceled) ||
		errors.Is(authorizationErr, context.DeadlineExceeded):
		status = mcpserver.AuthorizationCanceled
	case authorizationErr != nil:
		status = mcpserver.AuthorizationFailed
	}
	if err := attempt.Finish(status, service.now()); err == nil {
		writeContext, cancelWrite := context.WithTimeout(context.WithoutCancel(service.lifetime), 5*time.Second)
		writeErr := service.store.PutMCPAuthorizationAttempt(writeContext, attempt)
		cancelWrite()
		if writeErr != nil {
			service.logger.Error("persist MCP authorization outcome", "server", name, "error", writeErr)
		}
	}
	if current {
		service.publish(name)
	}
	if watch {
		go service.watchSession(name, generation, session)
	}
}
