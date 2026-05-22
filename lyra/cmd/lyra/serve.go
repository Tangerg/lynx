package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	lyrahttp "github.com/Tangerg/lynx/lyra/internal/transport/http"
)

// ServeCmd is `lyra serve [--listen :8080]` — boot the HTTP+SSE
// transport. This is the first Phase-2 transport realising the
// "server runtime" promise from ARCHITECTURE.md: web / desktop
// clients connect over /v1/agent/run (SSE) and CRUD sessions via
// /v1/sessions.
//
// Shutdown: SIGINT / SIGTERM trigger a graceful 10s drain — the
// SSE in-flight turns get a chance to finish their TurnEnd event
// before the listener closes.
func (a *App) ServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Lyra as an HTTP+SSE server.",
		Long: `Boot the HTTP+SSE transport so web / desktop clients can drive
turns over /v1/agent/run and manage sessions over /v1/sessions.

Routes:
  POST /v1/agent/run            — start a turn; response is AG-UI SSE
  POST /v1/turns/{id}/steer     — inject mid-turn user steering
  GET  /v1/sessions             — list sessions
  POST /v1/sessions             — create a session
  GET  /v1/sessions/{id}        — fetch one session
  DEL  /v1/sessions/{id}        — delete a session
  GET  /v1/approvals            — list pending approval requests
  POST /v1/approvals/{id}       — decide a pending request
  GET  /v1/approvals/mode       — read the runtime approval mode
  POST /v1/approvals/mode       — switch the approval mode
  GET  /healthz                 — liveness probe`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensureRuntime(); err != nil {
				return a.fatalErr(err)
			}
			srv, err := lyrahttp.NewServer(lyrahttp.Config{
				Chat:     a.chat,
				Session:  a.session,
				Approval: a.approval,
				Addr:     addr,
			})
			if err != nil {
				return a.fatalErr(err)
			}
			return a.runServer(cmd.Context(), srv, addr)
		},
	}
	cmd.Flags().StringVar(&addr, "listen", ":8080", "address to bind (host:port)")
	return cmd
}

// runServer launches the listener in a goroutine, then blocks on
// either a listener error or a shutdown signal. Returns nil on
// clean shutdown so cobra reports exit code 0.
func (a *App) runServer(ctx context.Context, srv *lyrahttp.Server, addr string) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listenErr := make(chan error, 1)
	go func() {
		fmt.Fprintf(a.Err, "[lyra] http listening on %s\n", addr)
		listenErr <- srv.Start()
	}()

	select {
	case err := <-listenErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return a.fatalErr(err)
	case <-ctx.Done():
		fmt.Fprintln(a.Err, "[lyra] shutdown requested, draining...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return a.fatalErr(err)
	}
	// Drain the listenErr send from the goroutine — ListenAndServe
	// returns http.ErrServerClosed after Shutdown completes.
	if err := <-listenErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return a.fatalErr(err)
	}
	return nil
}
