package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runtimeownership"
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	runtimeserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// InstanceConfig is the exact host snapshot required to open one complete
// Runtime instance. Callers resolve environment and working-directory defaults
// before this boundary; the instance never rediscovers host paths.
type InstanceConfig struct {
	UserHome             string
	DefaultWorkspacePath string
	DataDirectory        string
	ConfigDirectories    []string
	BuildID              string
	ServerInfo           protocol.ServerInfo
}

// Instance owns one complete Runtime: its operation endpoint, workers,
// application Host and persistence graph.
type Instance struct {
	endpoint            *operation.Endpoint
	serverInfo          protocol.ServerInfo
	service             *runtimeserver.Server
	host                *Host
	stopRuntime         context.CancelFunc
	schedulerDone       <-chan struct{}
	databaseChangesDone <-chan struct{}
	recoveryDone        <-chan struct{}

	closeMu  sync.Mutex
	stopping bool
	closed   bool
}

const instanceShutdownTimeout = 10 * time.Second

const ownershipRecoveryInterval = time.Second

// OpenInstance serializes canonical data-directory setup, opens persistence,
// then releases that setup boundary before assembling and recovering one Host.
// Runtime processes may subsequently share the directory; finer application
// ownership prevents conflicting execution and recovery.
func OpenInstance(ctx context.Context, cfg InstanceConfig) (_ *Instance, _ config.Settings, err error) {
	if err := cfg.validate(); err != nil {
		return nil, config.Settings{}, err
	}
	setup, err := runtimeownership.PrepareDataDirectory(ctx, cfg.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	setupOwned := true
	defer func() {
		if setupOwned {
			err = errors.Join(err, setup.Release())
		}
	}()
	cfg.DataDirectory = setup.Directory

	buildID := cfg.BuildID
	if buildID == "" {
		buildID, err = ExecutableBuildID()
		if err != nil {
			return nil, config.Settings{}, err
		}
	}
	settings, err := LoadConfig(cfg.ConfigDirectories)
	if err != nil {
		return nil, config.Settings{}, err
	}
	client, err := DefaultClient(settings)
	if err != nil {
		return nil, config.Settings{}, err
	}

	stores, err := persistence.Open(ctx, persistence.Config{
		DataDirectory:        cfg.DataDirectory,
		DefaultWorkspacePath: cfg.DefaultWorkspacePath,
	})
	if err != nil {
		return nil, config.Settings{}, err
	}
	idempotencyNamespace := stores.IdempotencyNamespace
	storesOwned := true
	defer func() {
		if storesOwned {
			err = errors.Join(err, stores.Close())
		}
	}()

	providers := ProviderRegistry(stores.Providers)
	if err = SeedConfiguredProvider(ctx, providers, settings); err != nil {
		return nil, config.Settings{}, err
	}
	if err = SeedUtilityRole(ctx, stores.UtilityRole, settings); err != nil {
		return nil, config.Settings{}, err
	}
	mcpServers, err := MCPServers(settings.MCPServers)
	if err != nil {
		return nil, config.Settings{}, err
	}
	if err = SeedMCPServers(ctx, stores.MCPServers, mcpServers); err != nil {
		return nil, config.Settings{}, err
	}
	if err = setup.Release(); err != nil {
		return nil, config.Settings{}, err
	}
	setupOwned = false

	hookResolver := NewHookResolver(cfg.UserHome, stores.Trust)
	assemblyConfig := ComposeConfig(settings, stores, client, providers, hookResolver, buildID)
	ownership, err := runtimeownership.New(stores.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	assemblyConfig.SessionOwnership = ownership
	assemblyConfig.GoalDriveOwnership = ownership
	assemblyConfig.RecoveryOwnership = ownership
	assemblyConfig.UserHome = cfg.UserHome
	assemblyConfig.DefaultWorkspacePath = cfg.DefaultWorkspacePath
	assembly := NewAssembly(assemblyConfig)
	storesOwned = false
	defer func() { err = errors.Join(err, CloseAssembly(assembly)) }()

	host, err := BuildAssembly(ctx, assembly)
	if err != nil {
		return nil, config.Settings{}, err
	}
	hostOwned := true
	defer func() {
		if hostOwned {
			err = errors.Join(err, host.Close())
		}
	}()
	if err = RecoverStartup(ctx, host.Stack); err != nil {
		return nil, config.Settings{}, err
	}

	serverInfo := cfg.ServerInfo
	if serverInfo.Name == "" {
		serverInfo.Name = "runtime"
	}
	if serverInfo.Version == "" {
		serverInfo.Version = "dev"
	}
	serverInfo.Home = cfg.UserHome
	serverInfo.DefaultWorkspace = protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}
	service, err := newOperationService(host.Stack, serverInfo, idempotencyNamespace)
	if err != nil {
		return nil, config.Settings{}, err
	}

	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	endpoint := operation.New(service, operation.Config{
		IdempotencyStore:     host.Stack.IdempotencyStore,
		IdempotencyNamespace: idempotencyNamespace,
		Lifetime:             runtimeContext,
	})
	databaseChangesDone, err := stores.StartExternalChangeObserver(runtimeContext, func() {
		host.Stack.PublishInvalidation.Notify(invalidation.Notice{Resource: invalidation.Resync})
	})
	if err != nil {
		stopRuntime()
		return nil, config.Settings{}, err
	}
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		host.Stack.ScheduleFiring.RunWorker(runtimeContext)
	}()
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		runOwnershipRecovery(runtimeContext, host.Stack)
	}()

	instance := &Instance{
		endpoint:            endpoint,
		serverInfo:          serverInfo,
		service:             service,
		host:                host,
		stopRuntime:         stopRuntime,
		schedulerDone:       schedulerDone,
		databaseChangesDone: databaseChangesDone,
		recoveryDone:        recoveryDone,
	}
	hostOwned = false
	return instance, settings, nil
}

func (cfg InstanceConfig) validate() error {
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "user home", value: cfg.UserHome},
		{name: "default workspace path", value: cfg.DefaultWorkspacePath},
		{name: "data directory", value: cfg.DataDirectory},
	} {
		if path.value == "" {
			return fmt.Errorf("runtime: %s is required", path.name)
		}
		if !filepath.IsAbs(path.value) {
			return fmt.Errorf("runtime: %s must be absolute", path.name)
		}
	}
	if len(cfg.ConfigDirectories) == 0 {
		return errors.New("runtime: at least one config directory is required")
	}
	for _, directory := range cfg.ConfigDirectories {
		if directory == "" || !filepath.IsAbs(directory) {
			return errors.New("runtime: config directories must be non-empty absolute paths")
		}
	}
	return nil
}

// Endpoint returns the instance-owned binding-neutral operation entrypoint.
// Public bindings keep it private and expose only their typed methods.
func (i *Instance) Endpoint() *operation.Endpoint {
	if i == nil {
		return nil
	}
	return i.endpoint
}

// ServerInfo returns the immutable identity advertised by every binding.
func (i *Instance) ServerInfo() protocol.ServerInfo {
	if i == nil {
		return protocol.ServerInfo{}
	}
	return i.serverInfo
}

// Close stops admissions, cancels instance-owned operations and workers, joins
// the application Host. A timed-out dependency remains owned and a later Close
// resumes teardown.
func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	i.closeMu.Lock()
	defer i.closeMu.Unlock()
	if i.closed {
		return nil
	}
	if !i.stopping {
		i.stopping = true
		i.service.Close()
		i.endpoint.BeginShutdown()
		i.stopRuntime()
	}

	waitContext, cancel := context.WithTimeout(context.Background(), instanceShutdownTimeout)
	defer cancel()
	if err := i.endpoint.AwaitShutdown(waitContext); err != nil {
		return err
	}
	select {
	case <-i.schedulerDone:
	case <-waitContext.Done():
		return waitContext.Err()
	}
	if i.databaseChangesDone != nil {
		select {
		case <-i.databaseChangesDone:
		case <-waitContext.Done():
			return waitContext.Err()
		}
	}
	if i.recoveryDone != nil {
		select {
		case <-i.recoveryDone:
		case <-waitContext.Done():
			return waitContext.Err()
		}
	}
	if err := i.host.Close(); err != nil {
		return err
	}
	i.closed = true
	return nil
}

// runOwnershipRecovery detects process death by attempting the same kernel
// leases held by live Run and Goal owners. A contended lease is definitive
// liveness evidence and the recovery use cases skip that Session; an abandoned
// owner becomes recoverable immediately after the OS releases its descriptors,
// without clocks, heartbeats, or lease-expiry guesses.
func runOwnershipRecovery(ctx context.Context, stack Stack) {
	ticker := time.NewTicker(ownershipRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if stack.OwnershipRecovery != nil {
				_, _ = stack.OwnershipRecovery.Reconcile(ctx)
			}
		}
	}
}
