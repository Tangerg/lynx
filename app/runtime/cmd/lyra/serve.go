package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/observability"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
)

func run(ctx context.Context, errw io.Writer) (err error) {
	shutdownObs := observability.Setup(resolvedVersion())
	defer func() { err = errors.Join(err, shutdownObs(context.WithoutCancel(ctx))) }()

	host, cfg, err := bootstrapRuntime(ctx)
	if err != nil {
		return err
	}
	// The Host owns the application tier's reverse-order shutdown (§10.3): the
	// integrations reconcile + codebase reindex tasks, then the run pump + engine +
	// persistence. api.Close (the run supervisor) is deferred later, so LIFO runs
	// it first — transport → supervisor → reconciler → engine/persistence.
	defer func() { err = errors.Join(err, host.Close()) }()
	srv := cfg.Server
	if len(srv.CORSOrigins) == 0 {
		srv.CORSOrigins = lyrahttp.DefaultCORSOrigins
	}
	if srv.Listen == "" {
		return errors.New("server.listen is empty (set config server.listen or LYRA_SERVER_LISTEN)")
	}
	// Re-drive durable startup recovery before constructing a delivery adapter, so
	// no transport can observe a session whose working tree and history disagree.
	if err := bootstrap.RecoverStartup(ctx, host.Stack); err != nil {
		return err
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
	httpServer, api, err := buildHTTPServer(host.Stack, srv, tokenValue)
	if err != nil {
		return err
	}
	defer api.Close()
	return runServer(ctx, errw, httpServer, host.Stack.Schedules.RunWorker, srv.Listen, token)
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved settings.
func buildHTTPServer(stack bootstrap.Stack, srv config.ServerConfig, tokenValue string) (*lyrahttp.Server, *server.Server, error) {
	info := lyrahttp.ServerInfoOrDefault()
	info.Version = resolvedVersion()
	if home, err := os.UserHomeDir(); err == nil {
		info.Cwd = home
		info.Home = home
	}

	api, err := server.New(server.Config{
		Sessions:     stack.Sessions,
		Integrations: stack.Integrations,
		Approvals:    stack.Approvals,
		Models:       stack.Models,
		Tools:        stack.Tools,
		Codebase:     stack.Codebase,
		ServerInfo:   info,
		// The run coordinator is built + owned by the Host; delivery drives it as a
		// use-case surface. Its file-change nudges reach the delivery workspace hub
		// through the notifier the Server observes.
		Coordinator:        stack.Coordinator,
		FileChanges:        stack.FileChanges,
		MCPStatus:          stack.MCPStatus,
		SkillChanges:       stack.SkillChanges,
		ScheduleFires:      stack.ScheduleFires,
		Queries:            stack.Queries,
		Usage:              stack.Usage,
		Schedules:          stack.Schedules,
		Goals:              stack.Goals,
		AgentMemory:        stack.AgentMemory,
		WorkspaceRoots:     stack.WorkspaceRoots,
		WorkspaceFiles:     stack.WorkspaceFiles,
		WorkspaceVCS:       stack.WorkspaceVCS,
		WorkspaceDiscovery: stack.WorkspaceDiscovery,
		WorkspaceKnowledge: stack.WorkspaceKnowledge,
		WorkspaceSkills:    stack.WorkspaceSkills,
		WorkspaceHooks:     stack.WorkspaceHooks,
		WorkspaceWatch:     stack.WorkspaceWatch,
		GitAvailable:       stack.GitAvailable,
	})
	if err != nil {
		return nil, nil, err
	}

	httpServer, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:          api,
		Addr:             srv.Listen,
		ServerInfo:       info,
		ProtocolVersion:  protocol.ProtocolVersion,
		LocalToken:       tokenValue,
		CORSOrigins:      srv.CORSOrigins,
		IdempotencyStore: stack.IdempotencyStore,
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

// runServer launches the server, blocks until it returns or a shutdown signal
// arrives, then drains with a 10s budget.
func runServer(ctx context.Context, errw io.Writer, httpServer *lyrahttp.Server, runScheduler func(context.Context), addr string, token *lyrahttp.LocalToken) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	// The scheduled-run worker shares the server lifetime: it fires due
	// schedules as headless runs and is joined before process resources close.
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		runScheduler(ctx)
	}()
	defer func() {
		stop()
		<-schedulerDone
	}()

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
