package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/observability"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage"
)

func run(ctx context.Context, errw io.Writer) (err error) {
	shutdownObs := observability.Setup(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownObs(context.WithoutCancel(ctx))) }()

	stack, cfg, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stack.Runtime.Close()) }()
	srv := cfg.Server
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = lyrahttp.DefaultCORSOrigins
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or LYRA_SERVER_LISTEN)")
	}

	var token *lyrahttp.LocalToken
	if !srv.NoLocalToken {
		t, err := lyrahttp.IssueLocalToken(srv.LocalTokenPath)
		if err != nil {
			return err
		}
		token = t
	}

	tokenValue := ""
	if token != nil {
		tokenValue = token.Value
	}
	httpServer, api, err := buildHTTPServer(stack, srv, tokenValue)
	if err != nil {
		return err
	}
	defer api.Close()
	return runServer(ctx, errw, httpServer, api, srv.Listen, token)
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved settings.
func buildHTTPServer(stack bootstrap.Stack, srv config.ServerConfig, tokenValue string) (*lyrahttp.Server, *server.Server, error) {
	info := lyrahttp.ServerInfoOrDefault()
	info.Version = resolvedVersion()
	if home, err := os.UserHomeDir(); err == nil {
		info.Cwd = home
		info.Home = home
	}

	// File checkpoints live in a shadow git repo per session under the lyra
	// home; the checkpoint adapter enables them only when git is present.
	var checkpointDir string
	if home, err := storage.Home(); err == nil {
		checkpointDir = filepath.Join(home, "checkpoints")
	}

	api, err := server.New(server.Config{
		Runtime:      stack.Runtime,
		Sessions:     stack.Sessions,
		Capabilities: stack.Capabilities,
		ServerInfo:   info,
		Checkpoints:  workspace.NewCheckpoints(checkpointDir),
		Schedules:    stack.Schedules,
		Workspace:    stack.Workspace,
	})
	if err != nil {
		return nil, nil, err
	}

	caps := server.Capabilities(stack.Capabilities, stack.Workspace.HasMemory())
	httpServer, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         api,
		Addr:            srv.Listen,
		ServerInfo:      info,
		ProtocolVersion: caps.ProtocolVersion,
		Capabilities:    caps,
		LocalToken:      tokenValue,
		CORSOrigins:     srv.CORSOrigins,
		HealthProbes: []lyrahttp.HealthProbe{
			{
				Name: "runtime",
				Probe: func(ctx context.Context) lyrahttp.HealthCheck {
					if err := api.Ping(ctx); err != nil {
						return lyrahttp.HealthCheck{Status: lyrahttp.HealthUnhealthy, Detail: err.Error()}
					}
					return lyrahttp.HealthCheck{Status: lyrahttp.HealthOK}
				},
			},
		},
		AgentDocsLister: agentDocsLister(),
	})
	if err != nil {
		api.Close()
		return nil, nil, err
	}
	return httpServer, api, nil
}

// resolvedVersion keeps HTTP identity and telemetry resource metadata aligned:
// an explicit link-time version wins, then Go module build info, then "dev".
func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	return lyrahttp.ServerInfoOrDefault().Version
}

// agentDocsLister returns an AgentDocsLister wired to the server's working
// directory (process cwd at construction time, locked once so a later chdir
// doesn't shift discovery to a different tree).
func agentDocsLister() lyrahttp.AgentDocsLister {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	return func(ctx context.Context) []lyrahttp.AgentDocInfo {
		files, err := agentdoc.Discover(ctx, cwd, home)
		if err != nil {
			return nil
		}
		out := make([]lyrahttp.AgentDocInfo, 0, len(files))
		for _, f := range files {
			out = append(out, lyrahttp.AgentDocInfo{
				Path:  f.Path,
				Bytes: len(f.Content),
			})
		}
		return out
	}
}

// runServer launches the server, blocks until it returns or a shutdown signal
// arrives, then drains with a 10s budget.
func runServer(ctx context.Context, errw io.Writer, httpServer *lyrahttp.Server, api *server.Server, addr string, token *lyrahttp.LocalToken) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	// The scheduled-run worker shares the server lifetime: it fires due
	// schedules as headless runs and is joined before process resources close.
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		api.RunScheduler(ctx)
	}()
	defer func() {
		stop()
		<-schedulerDone
	}()

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(errw, "[lyra] http listening on %s\n", addr)
		fmt.Fprintf(errw, "[lyra]   POST /v2/rpc/{method}     JSON-RPC (streaming methods -> text/event-stream)\n")
		fmt.Fprintf(errw, "[lyra]   GET  /v2/info             metadata (no auth)\n")
		fmt.Fprintf(errw, "[lyra]   GET  /v2/health           liveness\n")
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
