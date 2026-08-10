package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/telemetry"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

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
		srv.CORSOrigins = lyrahttp.DefaultCORSOrigins()
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or LYRA_SERVER_LISTEN)")
	}
	var token *lyrahttp.LocalToken
	if !srv.NoLocalToken {
		tokenPath := srv.LocalTokenPath
		if tokenPath == "" {
			tokenPath = filepath.Join(paths.dataDirectory, "local-token")
		}
		t, err := lyrahttp.IssueLocalToken(tokenPath)
		if err != nil {
			return err
		}
		token = t
	}

	tokenValue := ""
	if token != nil {
		tokenValue = token.Value
	}
	httpServer, err := buildHTTPServer(instance, srv, tokenValue)
	if err != nil {
		return err
	}
	return runServer(ctx, errw, httpServer, srv.Listen, token)
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved settings.
func buildHTTPServer(instance *bootstrap.Instance, srv config.Server, tokenValue string) (*lyrahttp.Server, error) {
	info := instance.ServerInfo()
	return lyrahttp.NewServer(lyrahttp.Config{
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
	return lyrahttp.ServerInfoOrDefault().Version
}

// runServer launches the server, blocks until it returns or a shutdown signal
// arrives, then drains with a 10s budget.
func runServer(ctx context.Context, errw io.Writer, httpServer *lyrahttp.Server, addr string, token *lyrahttp.LocalToken) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(errw, "[lyra] http listening on %s\n", addr)
		fmt.Fprintf(errw, "[lyra]   POST /v2/rpc              JSON-RPC (streaming methods -> text/event-stream)\n")
		fmt.Fprintf(errw, "[lyra]   GET  /v2/info             metadata (no auth)\n")
		fmt.Fprintf(errw, "[lyra]   GET  /v2/health/live      liveness\n")
		fmt.Fprintf(errw, "[lyra]   GET  /v2/health/ready     dependency readiness\n")
		if token != nil {
			fmt.Fprintf(errw, "[lyra] local-token gate active; token at %s\n", token.Path)
		} else {
			fmt.Fprintln(errw, "[lyra] local-token gate disabled")
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
		fmt.Fprintln(errw, "[lyra] shutdown requested, draining...")
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
