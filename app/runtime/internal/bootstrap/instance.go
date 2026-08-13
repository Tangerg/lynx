package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
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

// Instance owns one complete Runtime: its operation endpoint, scheduler,
// application Host, persistence graph and canonical data-directory lease.
type Instance struct {
	endpoint   *operation.Endpoint
	serverInfo protocol.ServerInfo
	service    *runtimeserver.Server
	host       *Host
	lease      *dataDirectoryLease

	stopRuntime   context.CancelFunc
	schedulerDone <-chan struct{}

	closeMu  sync.Mutex
	stopping bool
	closed   bool
}

const instanceShutdownTimeout = 10 * time.Second

// OpenInstance acquires the canonical data directory, opens persistence,
// assembles and recovers the application Host, constructs the one operation
// endpoint, then starts Runtime-owned workers. Every failed step rolls back in
// reverse order and releases the directory lease last.
func OpenInstance(ctx context.Context, cfg InstanceConfig) (_ *Instance, _ config.Settings, err error) {
	if err := cfg.validate(); err != nil {
		return nil, config.Settings{}, err
	}
	lease, err := acquireDataDirectoryLease(cfg.DataDirectory)
	if err != nil {
		return nil, config.Settings{}, err
	}
	leaseOwned := true
	defer func() {
		if leaseOwned {
			err = errors.Join(err, lease.release())
		}
	}()
	cfg.DataDirectory = lease.directory

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

	hookResolver := NewHookResolver(cfg.UserHome, stores.Trust)
	assemblyConfig := ComposeConfig(settings, stores, client, providers, hookResolver, buildID)
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
	service, err := newOperationService(host.Stack, serverInfo)
	if err != nil {
		return nil, config.Settings{}, err
	}

	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	endpoint := operation.New(service, operation.Config{
		IdempotencyStore: host.Stack.IdempotencyStore,
		Lifetime:         runtimeContext,
	})
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		host.Stack.ScheduleFiring.RunWorker(runtimeContext)
	}()

	instance := &Instance{
		endpoint:      endpoint,
		serverInfo:    serverInfo,
		service:       service,
		host:          host,
		lease:         lease,
		stopRuntime:   stopRuntime,
		schedulerDone: schedulerDone,
	}
	hostOwned = false
	leaseOwned = false
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
// the application Host, then releases the canonical data-directory lease. A
// timed-out dependency remains owned and a later Close resumes teardown.
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
	if err := i.host.Close(); err != nil {
		return err
	}
	if err := i.lease.release(); err != nil {
		return err
	}
	i.closed = true
	return nil
}
