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
	lyraipc "github.com/Tangerg/lynx/lyra/internal/transport/ipc"
)

// ServeCmd is `lyra serve` — boot one or more transport adapters
// against the same in-process runtime bundle.
//
// Flags:
//
//	--listen :8080  HTTP+SSE transport (set to "" to disable)
//	--stdio         stdio JSON-RPC transport on stdin/stdout
//
// At least one transport must be enabled. The runtime is shared
// across them — sessions / approvals / events flow through the
// same engine regardless of which transport the client connects on.
//
// Shutdown: SIGINT / SIGTERM triggers graceful drain on every
// running transport.
func (a *App) ServeCmd() *cobra.Command {
	var (
		httpAddr  string
		ipcStdio  bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Lyra as a server — HTTP+SSE and/or stdio IPC.",
		Long: `Boot Lyra as a server. The same in-process runtime backs every
enabled transport — sessions / approvals / events stay coherent
regardless of which transport a client connects on.

Transports:
  --listen :8080   HTTP+SSE (AG-UI compatible). "" disables.
  --stdio          stdio JSON-RPC on stdin/stdout.

HTTP routes:
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
  GET  /healthz                 — liveness probe

stdio JSON-RPC methods (newline-delimited JSON):
  agent.run / agent.steer / agent.cancel
  sessions.list / .create / .get / .delete
  approvals.list / .decide / .getMode / .setMode
  healthz`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if httpAddr == "" && !ipcStdio {
				return a.fatalErr(errors.New("at least one transport must be enabled (--listen or --stdio)"))
			}
			if err := a.ensureRuntime(); err != nil {
				return a.fatalErr(err)
			}
			return a.runTransports(cmd.Context(), httpAddr, ipcStdio)
		},
	}
	cmd.Flags().StringVar(&httpAddr, "listen", ":8080", `HTTP+SSE bind address ("" disables)`)
	cmd.Flags().BoolVar(&ipcStdio, "stdio", false, "enable stdio JSON-RPC transport")
	return cmd
}

// runTransports launches every enabled transport in its own
// goroutine and blocks until any of them returns or a shutdown
// signal arrives. Shutdown drains in parallel with a 10s budget.
func (a *App) runTransports(ctx context.Context, httpAddr string, ipcStdio bool) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	type fail struct{ name string; err error }
	failures := make(chan fail, 2)

	var httpServer *lyrahttp.Server
	if httpAddr != "" {
		srv, err := lyrahttp.NewServer(lyrahttp.Config{
			Runtime: a.runtime(),
			Addr:    httpAddr,
		})
		if err != nil {
			return a.fatalErr(err)
		}
		httpServer = srv
		go func() {
			fmt.Fprintf(a.Err, "[lyra] http listening on %s\n", httpAddr)
			err := srv.Start()
			failures <- fail{name: "http", err: err}
		}()
	}

	if ipcStdio {
		srv, err := lyraipc.NewServer(lyraipc.Config{
			Runtime: a.runtime(),
			In:      a.In,
			Out:     a.Out,
		})
		if err != nil {
			return a.fatalErr(err)
		}
		go func() {
			fmt.Fprintln(a.Err, "[lyra] stdio JSON-RPC ready")
			err := srv.Serve(ctx)
			failures <- fail{name: "ipc", err: err}
		}()
	}

	// Block on first failure or shutdown signal.
	select {
	case f := <-failures:
		// http.ErrServerClosed is the clean-shutdown sentinel.
		if errors.Is(f.err, http.ErrServerClosed) || f.err == nil {
			break
		}
		return a.fatalErr(fmt.Errorf("%s transport: %w", f.name, f.err))
	case <-ctx.Done():
		fmt.Fprintln(a.Err, "[lyra] shutdown requested, draining...")
	}

	// Drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(shutdownCtx)
	}
	// IPC has no explicit shutdown — closing stdin via ctx cancel
	// is the natural drain. The goroutine's Serve returns when ctx
	// fires; the failures channel may receive after this select.

	return nil
}
