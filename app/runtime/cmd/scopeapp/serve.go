package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/bootstrap"
	"github.com/Tangerg/scope/app/runtime/internal/config"
	scopeapphttp "github.com/Tangerg/scope/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/scope/app/runtime/internal/infra/telemetry"
	"github.com/Tangerg/scope/app/runtime/localruntime"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

const runtimeLogPrefix = "[scopeapp]"

func run(ctx context.Context, errw io.Writer) (err error) {
	shutdownTelemetry := telemetry.Configure(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownTelemetry(context.WithoutCancel(ctx))) }()

	instance, cfg, paths, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, instance.Close()) }()
	srv := cfg.Server
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = scopeapphttp.DefaultCORSOrigins()
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or SCOPEAPP_SERVER_LISTEN)")
	}
	var token *localruntime.Token
	if !srv.NoLocalToken {
		tokenPath := srv.LocalTokenPath
		if tokenPath == "" {
			tokenPath = paths.dataDirectory.LocalTokenPath()
		}
		t, openTokenErr := localruntime.OpenToken(tokenPath)
		if openTokenErr != nil {
			return openTokenErr
		}
		token = t
	}

	tokenValue := ""
	if token != nil {
		tokenValue = token.Value()
	}
	httpServer, err := buildHTTPServer(instance, srv, tokenValue)
	if err != nil {
		return err
	}
	return runServer(ctx, errw, httpServer, srv.Listen, token)
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved settings.
func buildHTTPServer(instance *bootstrap.Instance, srv config.Server, tokenValue string) (*scopeapphttp.Server, error) {
	info := instance.ServerInfo()
	return scopeapphttp.NewServer(scopeapphttp.Config{
		Endpoint:        instance.Endpoint(),
		Addr:            srv.Listen,
		ServerInfo:      info,
		ProtocolVersion: protocol.ProtocolVersion,
		LocalToken:      tokenValue,
		CORSOrigins:     srv.CORSOrigins,
	})
}

// resolvedVersion keeps HTTP identity and telemetry resource metadata aligned:
// an explicit link-time version wins, then Go module build info, then "dev".
func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	return scopeapphttp.ServerInfoOrDefault().Version
}

// runServer launches the server, blocks until it returns or a shutdown signal
// arrives, then drains with a 10s budget.
func runServer(ctx context.Context, errw io.Writer, httpServer *scopeapphttp.Server, addr string, token *localruntime.Token) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(errw, "%s http listening on %s\n", runtimeLogPrefix, addr)
		fmt.Fprintf(errw, "%s   POST /v2/rpc              JSON-RPC (streaming methods -> text/event-stream)\n", runtimeLogPrefix)
		fmt.Fprintf(errw, "%s   GET  /v2/info             metadata (no auth)\n", runtimeLogPrefix)
		fmt.Fprintf(errw, "%s   GET  /v2/health/live      liveness\n", runtimeLogPrefix)
		fmt.Fprintf(errw, "%s   GET  /v2/health/ready     dependency readiness\n", runtimeLogPrefix)
		if token != nil {
			fmt.Fprintf(errw, "%s local-token gate active; token at %s\n", runtimeLogPrefix, token.Path())
		} else {
			fmt.Fprintln(errw, runtimeLogPrefix+" local-token gate disabled")
		}
		errs <- httpServer.Start()
	}()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) || err == nil {
			return nil
		}
		return err
	case <-ctx.Done():
		fmt.Fprintln(errw, runtimeLogPrefix+" shutdown requested, draining...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	shutdownErr := httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, httpServer.Close())
	}
	serveErr := <-errs
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}
