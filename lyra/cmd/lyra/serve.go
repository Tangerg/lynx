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

	"github.com/Tangerg/lynx/lyra/pkg/coreimpl"
	lyrahttp "github.com/Tangerg/lynx/lyra/pkg/transport/http"
)

// ServeCmd is `lyra serve` — boot the JSON-RPC over HTTP transport
// that the frontend's Lyra Runtime Protocol talks to.
//
// Wire endpoints:
//
//	POST /v1/rpc[/{method}]   JSON-RPC Request / Notification
//	GET  /v1/rpc/stream       SSE — server-pushed notifications
//	GET  /v1/info             Flat-JSON server metadata (no auth)
//	GET  /v1/health           Liveness probe
//
// See docs/{API,TRANSPORT}.md for the full protocol.
func (a *App) ServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Lyra as a JSON-RPC over HTTP server.",
		Long: `Boot Lyra as a server. The HTTP transport surfaces the Lyra
Runtime Protocol on a single ` + "`/v1/rpc`" + ` endpoint plus an
` + "`/v1/rpc/stream`" + ` SSE channel, with ` + "`/v1/info`" + ` and
` + "`/v1/health`" + ` sidecars for operations.

Stdio transport is intentionally not supported — see docs/API.md §1.1.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if addr == "" {
				return a.fatalErr(errors.New("--listen is required"))
			}
			if err := a.ensureRuntime(); err != nil {
				return a.fatalErr(err)
			}

			api, err := coreimpl.New(coreimpl.Config{
				Runtime: a.runtime(),
				ServerInfo: lyrahttp.ServerInfoOrDefault(),
			})
			if err != nil {
				return a.fatalErr(err)
			}

			server, err := lyrahttp.NewServer(lyrahttp.Config{
				API:             api,
				Addr:            addr,
				ServerInfo:      lyrahttp.ServerInfoOrDefault(),
				ProtocolVersion: coreimpl.ProtocolVersion,
				Capabilities:    coreimpl.ServerCapabilities(),
			})
			if err != nil {
				return a.fatalErr(err)
			}

			return a.runServer(cmd.Context(), server, addr)
		},
	}
	cmd.Flags().StringVar(&addr, "listen", ":8080", "HTTP bind address")
	return cmd
}

// runServer launches the server, blocks until it returns or a
// shutdown signal arrives, then drains with a 10s budget.
func (a *App) runServer(ctx context.Context, server *lyrahttp.Server, addr string) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(a.Err, "[lyra] http listening on %s\n", addr)
		fmt.Fprintf(a.Err, "[lyra]   POST /v1/rpc[/{method}]   JSON-RPC\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/rpc/stream       SSE notifications\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/info             metadata (no auth)\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/health           liveness\n")
		errs <- server.Start()
	}()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) || err == nil {
			return nil
		}
		return a.fatalErr(err)
	case <-ctx.Done():
		fmt.Fprintln(a.Err, "[lyra] shutdown requested, draining...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return a.fatalErr(err)
	}
	return nil
}
