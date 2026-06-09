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

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
	"github.com/Tangerg/lynx/lyra/rpc/server"
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
)

// ServeCmd is `lyra serve` — boot the JSON-RPC over HTTP transport
// that the frontend's Lyra Runtime Protocol talks to.
//
// Wire endpoints (streamable HTTP — a streaming method's events ride its
// own POST response as text/event-stream; no separate stream endpoint):
//
//	POST /v2/rpc/{method}     JSON-RPC; streaming methods reply text/event-stream
//	GET  /v2/info             Flat-JSON server metadata (no auth)
//	GET  /v2/health           Liveness probe
//
// See docs/{API,TRANSPORT}.md for the full protocol.
func (a *App) ServeCmd() *cobra.Command {
	var (
		addr           string
		localTokenPath string
		noLocalToken   bool
		corsOrigins    []string
		a2aListen      string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Lyra as a JSON-RPC over HTTP server.",
		Long: `Boot Lyra as a server. The HTTP transport surfaces the Lyra
Runtime Protocol on a single ` + "`POST /v2/rpc/{method}`" + ` endpoint
(streamable HTTP: a streaming method's events ride its own response as
text/event-stream), with ` + "`/v2/info`" + ` and ` + "`/v2/health`" + `
sidecars for operations.

Stdio transport is intentionally not supported — see docs/API.md §1.1.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Wire the dev observability triad (traces + metrics + logs →
			// one slog stream) before anything else, so startup itself is
			// traced and every module's spans/logs are correlated.
			shutdownObs := setupObservability(version)
			defer shutdownObs(context.Background())

			if err := a.ensureRuntime(cmd.Context()); err != nil {
				return a.fatalErr(err)
			}
			// Server settings come from config (config/config.yaml);
			// CLI flags override per-field when explicitly set.
			srv := a.config().Server
			if cmd.Flags().Changed("listen") {
				srv.Listen = addr
			}
			if cmd.Flags().Changed("no-local-token") {
				srv.NoLocalToken = noLocalToken
			}
			if cmd.Flags().Changed("local-token-path") {
				srv.LocalTokenPath = localTokenPath
			}
			if cmd.Flags().Changed("cors-origin") {
				srv.CORSOrigins = corsOrigins
			}
			if cmd.Flags().Changed("a2a-listen") {
				srv.A2AListen = a2aListen
			}
			if len(srv.CORSOrigins) == 0 {
				srv.CORSOrigins = lyrahttp.DefaultCORSOrigins
			}
			if srv.Listen == "" {
				return a.fatalErr(errors.New("server.listen is empty (set serve --listen or config server.listen)"))
			}

			var token *lyrahttp.LocalToken
			if !srv.NoLocalToken {
				t, err := lyrahttp.IssueLocalToken(srv.LocalTokenPath)
				if err != nil {
					return a.fatalErr(err)
				}
				token = t
			}

			tokenValue := ""
			if token != nil {
				tokenValue = token.Value
			}
			httpServer, err := a.buildHTTPServer(srv, tokenValue)
			if err != nil {
				return a.fatalErr(err)
			}

			// Opt-in A2A endpoint on its own listener (separate protocol).
			var a2aServer *http.Server
			if srv.A2AListen != "" {
				a2aServer, err = a.buildA2AServer(srv.A2AListen, lyrahttp.ServerInfoOrDefault())
				if err != nil {
					return a.fatalErr(err)
				}
			}
			return a.runServer(cmd.Context(), httpServer, srv.Listen, token, a2aServer)
		},
	}
	cmd.Flags().StringVar(&addr, "listen", "",
		"HTTP bind address; overrides config server.listen (default 127.0.0.1:17171)")
	cmd.Flags().StringVar(&localTokenPath, "local-token-path", "",
		"local-process gate token path; overrides config server.localTokenPath")
	cmd.Flags().BoolVar(&noLocalToken, "no-local-token", false,
		"disable the local-process gate; overrides config server.noLocalToken")
	cmd.Flags().StringSliceVar(&corsOrigins, "cors-origin", nil,
		"CORS-allowed origin (repeatable); overrides config server.corsOrigins")
	cmd.Flags().StringVar(&a2aListen, "a2a-listen", "",
		"bind address for the opt-in A2A endpoint (exposes this agent to other agents); empty disables it")
	return cmd
}

// buildHTTPServer assembles the HTTP+SSE server from the resolved serve
// settings: the in-process protocol Runtime, the serve-process directory
// context the frontend reads on initialize (API.md §7.1 — cwd seeds a new
// session's default working dir, home anchors ~-scoped lookups; both
// default to the user's home), and a runtime health probe. tokenValue is
// the local-process gate token ("" when the gate is disabled).
func (a *App) buildHTTPServer(srv config.ServerConfig, tokenValue string) (*lyrahttp.Server, error) {
	info := lyrahttp.ServerInfoOrDefault()
	if home, err := os.UserHomeDir(); err == nil {
		info.Cwd = home
		info.Home = home
	}

	api, err := server.New(server.Config{
		Runtime:    a.runtime(),
		ServerInfo: info,
	})
	if err != nil {
		return nil, err
	}

	caps := server.Capabilities(a.runtime())
	return lyrahttp.NewServer(lyrahttp.Config{
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
}

// agentDocsLister returns an AgentDocsLister wired to the server's
// working directory (process cwd at construction time, locked once
// so a later `chdir` doesn't shift discovery to a different tree).
// Discovery walks the same paths the engine uses, so /v2/info's
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
func (a *App) runServer(ctx context.Context, server *lyrahttp.Server, addr string, token *lyrahttp.LocalToken, a2aServer *http.Server) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Buffered for both listeners so a failing one never blocks on send.
	errs := make(chan error, 2)
	go func() {
		fmt.Fprintf(a.Err, "[lyra] http listening on %s\n", addr)
		fmt.Fprintf(a.Err, "[lyra]   POST /v2/rpc/{method}     JSON-RPC (streaming methods → text/event-stream)\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v2/info             metadata (no auth)\n")
		fmt.Fprintf(a.Err, "[lyra]   GET  /v2/health           liveness\n")
		if token != nil {
			fmt.Fprintf(a.Err, "[lyra] local-token gate active; token at %s\n", token.Path)
		} else {
			fmt.Fprintln(a.Err, "[lyra] local-token gate disabled (--no-local-token)")
		}
		errs <- server.Start()
	}()

	if a2aServer != nil {
		fmt.Fprintf(a.Err, "[lyra] A2A endpoint on %s (POST %s, GET /.well-known/agent-card.json)\n", a2aServer.Addr, a2aRPCPattern)
		go func() { errs <- a2aServer.ListenAndServe() }()
	}

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
	if a2aServer != nil {
		_ = a2aServer.Shutdown(shutdownCtx)
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		return a.fatalErr(err)
	}
	return nil
}
