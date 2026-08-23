// Package runtimehost composes and owns one executable Runtime instance.
package runtimehost

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/agenttools"
	"github.com/Tangerg/lynx/app2/runtime/application"
	"github.com/Tangerg/lynx/app2/runtime/approvalflow"
	"github.com/Tangerg/lynx/app2/runtime/capabilityflow"
	"github.com/Tangerg/lynx/app2/runtime/checkpoint"
	"github.com/Tangerg/lynx/app2/runtime/codebaseflow"
	"github.com/Tangerg/lynx/app2/runtime/codeintel"
	"github.com/Tangerg/lynx/app2/runtime/compactionflow"
	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/goalflow"
	"github.com/Tangerg/lynx/app2/runtime/hookflow"
	"github.com/Tangerg/lynx/app2/runtime/hookfs"
	"github.com/Tangerg/lynx/app2/runtime/hookprocess"
	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/identity"
	"github.com/Tangerg/lynx/app2/runtime/interruptflow"
	"github.com/Tangerg/lynx/app2/runtime/localruntime"
	"github.com/Tangerg/lynx/app2/runtime/mcpflow"
	"github.com/Tangerg/lynx/app2/runtime/memoryflow"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/operationsflow"
	"github.com/Tangerg/lynx/app2/runtime/planflow"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/providerflow"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
	"github.com/Tangerg/lynx/app2/runtime/runtimeevents"
	"github.com/Tangerg/lynx/app2/runtime/scheduleflow"
	"github.com/Tangerg/lynx/app2/runtime/sessionflow"
	"github.com/Tangerg/lynx/app2/runtime/shellflow"
	"github.com/Tangerg/lynx/app2/runtime/sqlite"
	"github.com/Tangerg/lynx/app2/runtime/streamhub"
	"github.com/Tangerg/lynx/app2/runtime/toolflow"
	"github.com/Tangerg/lynx/app2/runtime/transcriptflow"
	"github.com/Tangerg/lynx/app2/runtime/workspaceflow"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type Config struct {
	Listen             string
	DatabasePath       string
	TokenPath          string
	DescriptorPath     string
	BootstrapNonce     string
	DefaultWorkspace   string
	UserHome           string
	ServerName         string
	ServerVersion      string
	CORSOrigins        []string
	Online             agenttools.OnlineConfig
	LSPServers         []codeintel.ServerSpec
	Remote             bool
	TLSCertificatePath string
	TLSPrivateKeyPath  string
	Logger             *slog.Logger
}

type Runtime struct {
	database           *sqlite.Database
	application        *application.Runtime
	endpoint           *operation.Endpoint
	server             *httptransport.Server
	listener           net.Listener
	descriptor         localruntime.Descriptor
	descriptorPath     string
	tokenPath          string
	tlsCertificatePath string
	tlsPrivateKeyPath  string
	shells             *shellflow.Service
	languageServers    *codeintel.Service
	compactions        *compactionflow.Service
	ephemeral          bool
	cancelLife         context.CancelFunc

	runMu     sync.Mutex
	ran       bool
	closing   bool
	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

func Open(ctx context.Context, config Config) (_ *Runtime, err error) {
	if ctx == nil {
		return nil, errors.New("runtimehost: context is required")
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	database, err := sqlite.Open(ctx, sqlite.Config{Path: config.DatabasePath, CreatedByVersion: config.ServerVersion})
	if err != nil {
		return nil, fmt.Errorf("runtimehost: open database: %w", err)
	}
	guard := newOpenGuard()
	guard.Add(database.Close)
	var cancelLife context.CancelFunc
	defer func() {
		if err != nil {
			if cancelLife != nil {
				cancelLife()
			}
			err = errors.Join(err, guard.Close())
		}
	}()

	if config.TokenPath != "" {
		_, err = localruntime.OpenToken(config.TokenPath)
		if err != nil {
			return nil, fmt.Errorf("runtimehost: open local token: %w", err)
		}
		if config.DescriptorPath != "" {
			guard.Add(func() error { return removeOwned(config.TokenPath) })
		}
	}
	instanceID, err := newInstanceID()
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	cancelLife = cancel
	shells, err := shellflow.New(lifetime)
	if err != nil {
		return nil, err
	}
	guard.AddClose(shells.Close)
	languageServers, err := codeintel.New(lifetime, config.LSPServers)
	if err != nil {
		return nil, err
	}
	guard.Add(languageServers.Close)
	enabledFeatures := make(map[string]bool, len(protocol.Features()))
	for _, feature := range protocol.Features() {
		enabledFeatures[feature.Key] = true
	}
	service, err := discovery.New(discovery.Config{
		ServerInfo: protocol.ServerInfo{
			InstanceID: instanceID, Name: config.ServerName, Version: config.ServerVersion,
			DefaultWorkspace: protocol.WorkspaceRef{Path: config.DefaultWorkspace}, Home: config.UserHome,
		},
		IdempotencyNamespace: database.Metadata().IdempotencyNamespace,
		EnabledFeatures:      enabledFeatures,
		RunEvents:            protocol.RunEventTypes(), RuntimeTopics: protocol.RuntimeTopics(),
		StreamingMethods: operation.Contract().StreamMethods(),
	})
	if err != nil {
		return nil, err
	}
	workspaceResolver, err := workspacefs.NewResolver(config.DefaultWorkspace)
	if err != nil {
		return nil, err
	}
	capabilities, err := capabilityflow.New(database, workspaceResolver, identity.Generator{}, config.UserHome)
	if err != nil {
		return nil, err
	}
	hookSource, err := hookfs.New(config.UserHome)
	if err != nil {
		return nil, err
	}
	hooks, err := hookflow.New(hookflow.Config{
		Store: database, Source: hookSource, Resolver: workspaceResolver,
		Commands: hookprocess.Executor{}, Lifetime: lifetime, Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(hooks.Close)
	checkpoints := checkpoint.NewStore(filepath.Join(filepath.Dir(config.DatabasePath), "checkpoints"))
	sessions, err := sessionflow.New(sessionflow.Config{
		Store: database, IDs: identity.Generator{}, Workspaces: workspaceResolver,
		Checkpoints: checkpoints,
	})
	if err != nil {
		return nil, err
	}
	providers, err := providerflow.New(database)
	if err != nil {
		return nil, err
	}
	events, err := runtimeevents.New(runtimeevents.Config{
		UserSkillsDirectory: filepath.Join(config.UserHome, ".lyra", "skills"),
		KnowledgeFiles:      capabilities,
		HookFiles:           hooks,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(events.Close)
	schedules, err := scheduleflow.New(scheduleflow.Config{
		Store: database, IDs: identity.Generator{}, Workspaces: workspaceResolver, Events: events,
	})
	if err != nil {
		return nil, err
	}
	memory, err := memoryflow.New(memoryflow.Config{
		Store: database, Resolver: workspaceResolver, IDs: identity.Generator{},
		Events: events, Embeddings: runtimeMemoryEmbedding{providers: providers},
		Maintenance: runtimeMemoryMaintenance{providers: providers},
		Lifetime:    lifetime, Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(memory.Close)
	goalSignals := goalflow.NewSignals()
	goals, err := goalflow.New(database, identity.Generator{}, goalSignals, events)
	if err != nil {
		return nil, err
	}
	plans, err := planflow.New(database, events)
	if err != nil {
		return nil, err
	}
	mcp, err := mcpflow.New(mcpflow.Config{
		Store: database, IDs: identity.Generator{}, Events: events,
		Lifetime: lifetime, Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(mcp.Close)
	approvals, err := approvalflow.New(approvalflow.Config{
		Store: database, Sessions: database, Events: events,
	})
	if err != nil {
		return nil, err
	}
	transcripts, err := transcriptflow.New(database)
	if err != nil {
		return nil, err
	}
	compactions, err := compactionflow.New(compactionflow.Config{
		Store: database, Models: runtimeCompactionModels{providers: providers}, Hooks: hooks,
		IDs: identity.Generator{}, Events: events, Lifetime: lifetime, Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(compactions.Close)
	agentToolCatalog, err := agenttools.New(agenttools.Config{
		Policy: approvals, MCP: mcp, Results: database,
		Goals: goals, Plans: plans, Schedules: schedules,
		Skills: runtimeSkillGateway{capabilities: capabilities, events: events},
		Memory: runtimeMemory{service: memory}, Conversations: transcripts, Hooks: hooks,
		Shells: shells, Online: config.Online, CodeIntel: languageServers,
	})
	if err != nil {
		return nil, err
	}
	executor, err := agentexec.New(agentexec.Config{
		Clients: providers, Tools: agentToolCatalog,
		Documents: runtimeAgentDocuments{capabilities: capabilities},
		Knowledge: runtimeKnowledgeDocuments{capabilities: capabilities},
		Memory:    runtimeMemory{service: memory}, Hooks: hooks,
		RuntimeContext: runtimeLiveContext{shells: shells, plans: plans},
	})
	if err != nil {
		return nil, err
	}
	hub := streamhub.New()
	runs, err := runflow.New(runflow.Config{
		Store: database, IDs: identity.Generator{}, Executor: executor,
		Models: providers, Hub: hub, Events: events, Lifetime: lifetime, Checkpoints: checkpoints,
		Memory: memory, Compaction: compactions, Hooks: hooks,
	})
	if err != nil {
		return nil, err
	}
	guard.AddClose(runs.Close)
	if err := runs.Recover(ctx); err != nil {
		return nil, fmt.Errorf("runtimehost: recover predecessor runs: %w", err)
	}
	if err := compactions.Recover(ctx); err != nil {
		return nil, fmt.Errorf("runtimehost: recover conversation compaction: %w", err)
	}
	memory.Recover()
	goalDriver, err := goalflow.NewDriver(goalflow.DriverConfig{Goals: goals, Runs: runs, Signals: goalSignals, Lifetime: lifetime})
	if err != nil {
		return nil, err
	}
	guard.AddClose(goalDriver.Close)
	if err := goalDriver.Recover(ctx); err != nil {
		return nil, fmt.Errorf("runtimehost: recover autonomous goals: %w", err)
	}
	workspace, err := workspaceflow.New(workspaceResolver)
	if err != nil {
		return nil, err
	}
	scheduleLauncher, err := scheduleflow.NewLauncher(workspaceResolver, runs)
	if err != nil {
		return nil, err
	}
	scheduleDispatcher, err := scheduleflow.NewDispatcher(scheduleflow.DispatcherConfig{
		Store: database, IDs: identity.Generator{}, Events: events, Runner: scheduleLauncher,
		Lifetime: lifetime, Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}
	guard.Add(scheduleDispatcher.Close)
	interrupts, err := interruptflow.New(database)
	if err != nil {
		return nil, err
	}
	codebase, err := codebaseflow.New(
		database,
		workspaceResolver,
		providers,
		identity.Generator{},
		events,
		lifetime,
		config.Logger,
	)
	if err != nil {
		return nil, err
	}
	guard.AddClose(codebase.Close)
	tools, err := toolflow.New(workspaceResolver)
	if err != nil {
		return nil, err
	}
	operations, err := operationsflow.New(database, identity.Generator{})
	if err != nil {
		return nil, err
	}
	app, err := application.New(application.Config{
		Discovery: service, Sessions: sessions, Providers: providers,
		Runs: runs, Workspace: workspace, Schedules: schedules, ScheduleDispatcher: scheduleDispatcher,
		Approvals: approvals, Interrupts: interrupts, Plans: plans, Goals: goals, GoalDriver: goalDriver, MCP: mcp,
		Capability: capabilities, Hooks: hooks, Memory: memory, Codebase: codebase, Tools: tools, Operations: operations, Events: events,
	})
	if err != nil {
		return nil, err
	}
	goalDriver.Start()
	replayStore, err := sqlite.NewIdempotencyStore(
		database,
		time.Duration(protocol.DefaultIdempotencyTTL)*time.Second,
	)
	if err != nil {
		return nil, err
	}
	endpoint, err := operation.New(app, operation.Config{
		Lifetime: lifetime, IdempotencyStore: replayStore,
		IdempotencyNamespace: database.Metadata().IdempotencyNamespace,
	})
	if err != nil {
		return nil, err
	}
	server, err := httptransport.New(httptransport.Config{
		Dispatcher:  dispatch.New(endpoint),
		ServerInfo:  protocol.ServerInfo{InstanceID: instanceID, Name: config.ServerName, Version: config.ServerVersion},
		BearerToken: tokenSource(config.TokenPath), CORSOrigins: config.CORSOrigins,
		Logger: config.Logger,
		HealthProbes: []httptransport.HealthProbe{
			{Name: "runtime", Check: func(context.Context) httptransport.HealthCheck {
				if endpoint.Ready() {
					return httptransport.HealthCheck{Status: httptransport.HealthOK}
				}
				return httptransport.HealthCheck{Status: httptransport.HealthUnhealthy}
			}},
			{Name: "storage", Check: func(probeCtx context.Context) httptransport.HealthCheck {
				if err := database.Ready(probeCtx); err != nil {
					return httptransport.HealthCheck{Status: httptransport.HealthUnhealthy, Detail: err.Error()}
				}
				return httptransport.HealthCheck{Status: httptransport.HealthOK}
			}},
		},
	})
	if err != nil {
		return nil, err
	}
	guard.Add(server.Close)
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return nil, fmt.Errorf("runtimehost: listen: %w", err)
	}
	guard.Add(listener.Close)
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.IP == nil {
		return nil, errors.New("runtimehost: listener must resolve to a TCP address")
	}
	if !config.Remote && !address.IP.IsLoopback() {
		return nil, errors.New("runtimehost: local mode requires a loopback listener")
	}
	scheme := "http"
	if config.TLSCertificatePath != "" {
		scheme = "https"
	}
	descriptor := localruntime.Descriptor{
		BootstrapVersion: localruntime.BootstrapVersion,
		Nonce:            config.BootstrapNonce, PID: os.Getpid(), InstanceID: instanceID,
		ProtocolVersion: protocol.ProtocolVersion,
		BaseURL:         (&url.URL{Scheme: scheme, Host: listener.Addr().String()}).String(),
		TokenPath:       config.TokenPath,
	}
	runtime := &Runtime{
		database: database, application: app, endpoint: endpoint, server: server, listener: listener,
		descriptor: descriptor, tokenPath: config.TokenPath, ephemeral: config.DescriptorPath != "",
		tlsCertificatePath: config.TLSCertificatePath, tlsPrivateKeyPath: config.TLSPrivateKeyPath,
		shells: shells, languageServers: languageServers, compactions: compactions,
		cancelLife: cancelLife, closed: make(chan struct{}),
	}
	if config.DescriptorPath != "" {
		runtime.descriptorPath = config.DescriptorPath
	}
	guard.Disarm()
	return runtime, nil
}

func (runtime *Runtime) BaseURL() string { return runtime.descriptor.BaseURL }

func (runtime *Runtime) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("runtimehost: context is required")
	}
	runtime.runMu.Lock()
	if runtime.ran || runtime.closing {
		runtime.runMu.Unlock()
		return errors.New("runtimehost: Run may be called only once and before Close")
	}
	runtime.ran = true

	served := make(chan error, 1)
	go func() {
		if runtime.tlsCertificatePath != "" {
			served <- runtime.server.ServeTLS(runtime.listener, runtime.tlsCertificatePath, runtime.tlsPrivateKeyPath)
			return
		}
		served <- runtime.server.Serve(runtime.listener)
	}()
	if runtime.descriptorPath != "" {
		if err := localruntime.Publish(runtime.descriptorPath, runtime.descriptor); err != nil {
			runtime.runMu.Unlock()
			_ = runtime.Close(context.Background())
			return errors.Join(err, <-served)
		}
	}
	runtime.runMu.Unlock()
	select {
	case err := <-served:
		return errors.Join(err, runtime.Close(context.Background()))
	case <-ctx.Done():
		closeErr := runtime.Close(context.Background())
		return errors.Join(closeErr, <-served)
	}
}

func (runtime *Runtime) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("runtimehost: context is required")
	}
	runtime.closeOnce.Do(func() {
		runtime.runMu.Lock()
		runtime.closing = true
		runtime.runMu.Unlock()
		go runtime.closeOwned()
	})
	select {
	case <-runtime.closed:
		return runtime.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runtime *Runtime) closeOwned() {
	defer close(runtime.closed)
	runtime.endpoint.BeginShutdown()
	runtime.cancelLife()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverErr := runtime.server.Shutdown(shutdownCtx)
	if serverErr != nil {
		serverErr = errors.Join(serverErr, runtime.server.Close())
	}
	listenerErr := runtime.listener.Close()
	if errors.Is(listenerErr, net.ErrClosed) {
		listenerErr = nil
	}
	endpointErr := runtime.endpoint.AwaitShutdown(shutdownCtx)
	runtime.application.Close()
	runtime.compactions.Close()
	runtime.shells.Close()
	languageServerErr := runtime.languageServers.Close()
	databaseErr := runtime.database.Close()
	var artifactErr error
	if runtime.ephemeral {
		artifactErr = errors.Join(removeOwned(runtime.descriptorPath), removeOwned(runtime.tokenPath))
	}
	runtime.closeErr = errors.Join(serverErr, listenerErr, endpointErr, languageServerErr, databaseErr, artifactErr)
}

func validateConfig(config Config) error {
	if config.Listen == "" || config.ServerName == "" || config.ServerVersion == "" {
		return errors.New("runtimehost: listen address and server identity are required")
	}
	for name, path := range map[string]string{
		"database": config.DatabasePath, "workspace": config.DefaultWorkspace, "user home": config.UserHome,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("runtimehost: %s path must be absolute", name)
		}
	}
	if config.TokenPath != "" && !filepath.IsAbs(config.TokenPath) {
		return errors.New("runtimehost: token path must be absolute")
	}
	if (config.TLSCertificatePath == "") != (config.TLSPrivateKeyPath == "") {
		return errors.New("runtimehost: TLS certificate and private key must be configured together")
	}
	if config.TLSCertificatePath != "" && (!filepath.IsAbs(config.TLSCertificatePath) || !filepath.IsAbs(config.TLSPrivateKeyPath)) {
		return errors.New("runtimehost: TLS certificate paths must be absolute")
	}
	if config.Remote {
		if config.TokenPath == "" || config.TLSCertificatePath == "" || len(config.CORSOrigins) == 0 {
			return errors.New("runtimehost: remote mode requires bearer token, TLS, and exact CORS origins")
		}
		certificate, certificateErr := os.Stat(config.TLSCertificatePath)
		privateKey, privateKeyErr := os.Stat(config.TLSPrivateKeyPath)
		if certificateErr != nil || privateKeyErr != nil || !certificate.Mode().IsRegular() || !privateKey.Mode().IsRegular() {
			return errors.New("runtimehost: TLS certificate and private key must be existing regular files")
		}
		if privateKey.Mode().Perm()&0o077 != 0 || privateKey.Mode().Perm()&0o400 == 0 {
			return errors.New("runtimehost: TLS private key must be owner-readable and inaccessible to group or other users")
		}
		if config.DescriptorPath != "" {
			return errors.New("runtimehost: remote mode forbids a local bootstrap descriptor")
		}
		for _, origin := range config.CORSOrigins {
			if origin == "*" {
				return errors.New("runtimehost: remote mode forbids wildcard CORS")
			}
		}
	} else if config.TLSCertificatePath != "" {
		return errors.New("runtimehost: TLS is available only in explicit remote mode")
	}
	if config.DescriptorPath == "" {
		if config.BootstrapNonce != "" {
			return errors.New("runtimehost: bootstrap nonce requires a descriptor path")
		}
		return nil
	}
	if !filepath.IsAbs(config.DescriptorPath) || config.BootstrapNonce == "" || config.TokenPath == "" {
		return errors.New("runtimehost: descriptor handoff requires absolute descriptor/token paths and a nonce")
	}
	if filepath.Dir(filepath.Clean(config.DescriptorPath)) != filepath.Dir(filepath.Clean(config.TokenPath)) {
		return errors.New("runtimehost: descriptor and token must share one private spawn root")
	}
	return nil
}

func tokenSource(path string) httptransport.TokenSource {
	if path == "" {
		return nil
	}
	return fileTokenSource{path: path}
}

func newInstanceID() (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("runtimehost: create instance identity: %w", err)
	}
	return "ins_" + base64.RawURLEncoding.EncodeToString(value), nil
}

func removeOwned(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
