package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
	"github.com/Tangerg/lynx/lyra/rpc/server"
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
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
	var (
		addr           string
		localTokenPath string
		noLocalToken   bool
		corsOrigins    []string
	)
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
			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}

			var token *lyrahttp.LocalToken
			if !noLocalToken {
				t, err := lyrahttp.IssueLocalToken(localTokenPath)
				if err != nil {
					return a.fatalErr(err)
				}
				token = t
			}

			api, err := server.New(server.Config{
				Runtime:    a.runtime(),
				ServerInfo: lyrahttp.ServerInfoOrDefault(),
			})
			if err != nil {
				return a.fatalErr(err)
			}

			tokenValue := ""
			if token != nil {
				tokenValue = token.Value
			}
			httpServer, err := lyrahttp.NewServer(lyrahttp.Config{
				Runtime:         api,
				Addr:            addr,
				ServerInfo:      lyrahttp.ServerInfoOrDefault(),
				ProtocolVersion: server.ProtocolVersion,
				Capabilities:    server.Capabilities(),
				LocalToken:      tokenValue,
				CORSOrigins:     corsOrigins,
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
				return a.fatalErr(err)
			}

			return a.runServer(cmd.Context(), httpServer, addr, token)
		},
	}
	cmd.Flags().StringVar(&addr, "listen", "127.0.0.1:17171",
		"HTTP bind address (matches frontend's default AGUI_BASE)")
	cmd.Flags().StringVar(&localTokenPath, "local-token-path", "",
		"path for the local-process gate token (default: $HOME/.lyra/local-token)")
	cmd.Flags().BoolVar(&noLocalToken, "no-local-token", false,
		"disable the local-process gate (dev / same-origin only)")
	cmd.Flags().StringSliceVar(&corsOrigins, "cors-origin", lyrahttp.DefaultCORSOrigins,
		"CORS-allowed origin; repeatable. Pass an empty value to disable CORS.")
	return cmd
}

// agentDocsLister returns an AgentDocsLister wired to the server's
// working directory (process cwd at construction time, locked once
// so a later `chdir` doesn't shift discovery to a different tree).
// Discovery walks the same paths the engine uses, so /v1/info's
// agentDocs field reflects exactly what the model will see.
func agentDocsLister() lyrahttp.AgentDocsLister {
	cwd, err := os.Getwd()
	if err != nil {
		// No usable cwd ⇒ omit the field entirely. Nil lister tells
		// the Server to skip rendering the key.
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

// runServer launches the server, blocks until it returns or a
// shutdown signal arrives, then drains with a 10s budget.
func (a *App) runServer(ctx context.Context, server *lyrahttp.Server, addr string, token *lyrahttp.LocalToken) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		fmt.Fprintf(a.Err, "[lyra] http listening on %s\n", addr)
		fmt.Fprintf(a.Err, "[lyra]   POST /v1/rpc[/{method}]   JSON-RPC\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/rpc/stream       SSE notifications\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/info             metadata (no auth)\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v1/health           liveness\n")
		if token != nil {
			fmt.Fprintf(a.Err, "[lyra] local-token gate active; token at %s\n", token.Path)
		} else {
			fmt.Fprintln(a.Err, "[lyra] local-token gate disabled (--no-local-token)")
		}
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
