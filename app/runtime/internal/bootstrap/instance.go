package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runtimeownership"
	"github.com/Tangerg/lynx/app/runtime/internal/completion"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
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
	delivery            operationDelivery
	serverInfo          protocol.ServerInfo
	host                *Host
	stopRuntime         context.CancelFunc
	schedulerDone       <-chan struct{}
	databaseChangesDone <-chan struct{}
	recoveryDone        <-chan struct{}

	closeMu  sync.Mutex
	stopping bool
	closed   bool
	// shutdownTimeout bounds each Close caller's wait. Zero uses the process
	// default; tests may shorten it without changing the owner generation.
	shutdownTimeout time.Duration
	shutdown        *instanceShutdownAttempt
}

type instanceShutdownAttempt struct {
	done      chan struct{}
	err       error
	completed bool
}

const instanceShutdownTimeout = 10 * time.Second

// OpenInstance serializes canonical data-directory setup, opens persistence,
// then releases that setup boundary before assembling and recovering one Host.
// Runtime processes may subsequently share the directory; finer application
// ownership prevents conflicting execution and recovery.
func OpenInstance(ctx context.Context, cfg InstanceConfig) (_ *Instance, _ config.Settings, err error) {
	if validateErr := cfg.validate(); validateErr != nil {
		return nil, config.Settings{}, validateErr
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
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	runtimeOwned := true
	defer func() {
		if runtimeOwned {
			stopRuntime()
		}
	}()
	assembly := NewAssembly(runtimeContext, assemblyConfig)
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
	if err = host.application.recoverStartup(ctx); err != nil {
		return nil, config.Settings{}, err
	}

	serverInfo := cfg.ServerInfo
	serverInfo.InstanceID = "runtime_" + uuid.NewString()
	if serverInfo.Name == "" {
		serverInfo.Name = "runtime"
	}
	if serverInfo.Version == "" {
		serverInfo.Version = "dev"
	}
	serverInfo.Home = cfg.UserHome
	serverInfo.DefaultWorkspace = protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}
	delivery, err := host.application.openOperationDelivery(
		runtimeContext,
		serverInfo,
		idempotencyNamespace,
	)
	if err != nil {
		return nil, config.Settings{}, err
	}
	databaseChangesDone, err := stores.StartExternalChangeObserver(
		runtimeContext,
		host.application.notifyExternalChange,
	)
	if err != nil {
		stopRuntime()
		delivery.beginShutdown()
		rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), instanceShutdownTimeout)
		defer cancelRollback()
		return nil, config.Settings{}, errors.Join(err, delivery.awaitShutdown(rollbackCtx))
	}
	workerJoins := host.application.startWorkers(runtimeContext)

	instance := &Instance{
		delivery:            delivery,
		serverInfo:          serverInfo,
		host:                host,
		stopRuntime:         stopRuntime,
		schedulerDone:       workerJoins.scheduler,
		databaseChangesDone: databaseChangesDone,
		recoveryDone:        workerJoins.recovery,
	}
	runtimeOwned = false
	hostOwned = false
	return instance, settings, nil
}

func (i InstanceConfig) validate() error {
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "user home", value: i.UserHome},
		{name: "default workspace path", value: i.DefaultWorkspacePath},
		{name: "data directory", value: i.DataDirectory},
	} {
		if path.value == "" {
			return fmt.Errorf("runtime: %s is required", path.name)
		}
		if !filepath.IsAbs(path.value) {
			return fmt.Errorf("runtime: %s must be absolute", path.name)
		}
	}
	if len(i.ConfigDirectories) == 0 {
		return errors.New("runtime: at least one config directory is required")
	}
	for _, directory := range i.ConfigDirectories {
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
	return i.delivery.endpoint
}

// ServerInfo returns the immutable identity advertised by every binding.
func (i *Instance) ServerInfo() protocol.ServerInfo {
	if i == nil {
		return protocol.ServerInfo{}
	}
	return i.serverInfo
}

// Close starts or joins the Instance-owned delivery-to-Host shutdown
// generation. Each caller has a bounded wait; its timeout cannot cancel the
// generation. A completed phase error permits a later Close attempt.
func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	i.closeMu.Lock()
	if i.closed {
		i.closeMu.Unlock()
		return nil
	}
	attempt := i.shutdown
	if attempt == nil || attempt.completed {
		attempt = &instanceShutdownAttempt{done: make(chan struct{})}
		i.shutdown = attempt
		ownerCtx := context.Background()
		if i.host != nil && i.host.lifetime != nil && i.host.lifetime.context != nil {
			ownerCtx = context.WithoutCancel(i.host.lifetime.context)
		}
		go runInstanceShutdown(ownerCtx, i, attempt)
	}
	timeout := i.shutdownTimeout
	i.closeMu.Unlock()
	if timeout <= 0 {
		timeout = instanceShutdownTimeout
	}
	waitContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := completion.Wait(waitContext, attempt.done); err != nil {
		return err
	}
	return attempt.err
}

func runInstanceShutdown(
	ownerCtx context.Context,
	instance *Instance,
	attempt *instanceShutdownAttempt,
) {
	instance.closeMu.Lock()
	begin := !instance.stopping
	if begin {
		instance.stopping = true
	}
	instance.closeMu.Unlock()
	if begin {
		instance.delivery.beginShutdown()
		if instance.stopRuntime != nil {
			instance.stopRuntime()
		}
	}

	if err := instance.delivery.awaitShutdown(ownerCtx); err != nil {
		finishInstanceShutdown(instance, attempt, false, err)
		return
	}
	for _, done := range []<-chan struct{}{
		instance.schedulerDone,
		instance.databaseChangesDone,
		instance.recoveryDone,
	} {
		if err := completion.Wait(ownerCtx, done); err != nil {
			finishInstanceShutdown(instance, attempt, false, err)
			return
		}
	}
	if err := awaitHostShutdown(ownerCtx, instance.host); err != nil {
		finishInstanceShutdown(instance, attempt, false, err)
		return
	}
	finishInstanceShutdown(instance, attempt, true, nil)
}

func finishInstanceShutdown(
	instance *Instance,
	attempt *instanceShutdownAttempt,
	closed bool,
	err error,
) {
	instance.closeMu.Lock()
	defer instance.closeMu.Unlock()
	attempt.err = err
	attempt.completed = true
	if closed {
		instance.closed = true
	}
	close(attempt.done)
}
