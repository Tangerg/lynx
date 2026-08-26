package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	runtimeserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestInstanceConfigRequiresExactAbsoluteHostPaths(t *testing.T) {
	valid := InstanceConfig{
		UserHome:             t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		DataDirectory:        t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*InstanceConfig)
	}{
		{name: "missing user home", mutate: func(cfg *InstanceConfig) { cfg.UserHome = "" }},
		{name: "relative workspace", mutate: func(cfg *InstanceConfig) { cfg.DefaultWorkspacePath = "workspace" }},
		{name: "relative data", mutate: func(cfg *InstanceConfig) { cfg.DataDirectory = "data" }},
		{name: "missing config directories", mutate: func(cfg *InstanceConfig) { cfg.ConfigDirectories = nil }},
		{name: "relative config directory", mutate: func(cfg *InstanceConfig) { cfg.ConfigDirectories = []string{"config"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.validate(); err == nil {
				t.Fatal("validate accepted an incomplete host snapshot")
			}
		})
	}
}

func TestOpenInstanceOwnsOneEndpointAndCanonicalDirectory(t *testing.T) {
	t.Setenv("LYRA_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("LYRA_MCP_SERVERS", "")
	t.Setenv("LYRA_A2A_AGENTS", "")
	t.Setenv("LYRA_A2A_RPC_ORIGINS", "")

	cfg := InstanceConfig{
		UserHome:             t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		DataDirectory:        t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
		BuildID:              "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ServerInfo: protocol.ServerInfo{
			Name: "test-runtime", Version: "test-version", InstanceID: "runtime_caller_owned",
		},
	}
	instance, _, err := OpenInstance(t.Context(), cfg)
	if err != nil {
		t.Fatalf("OpenInstance: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close() })

	result := instance.Endpoint().Invoke(t.Context(), "runtime.discover", struct{}{}, operation.Options{})
	if result.Failure != nil {
		t.Fatalf("runtime.discover: %v", result.Failure)
	}
	discovery, ok := result.Value.(*protocol.DiscoverResponse)
	if !ok || discovery.ServerInfo.Name != "test-runtime" || discovery.ServerInfo.Home != cfg.UserHome {
		t.Fatalf("runtime.discover result = %+v", result.Value)
	}
	if discovery.ServerInfo.InstanceID == "" || discovery.ServerInfo.InstanceID == cfg.ServerInfo.InstanceID {
		t.Fatalf("runtime.discover instance identity = %q, want a fresh Bootstrap-owned identity", discovery.ServerInfo.InstanceID)
	}
	namespace := discovery.Capabilities.Limits.Idempotency.Namespace
	if namespace == "" {
		t.Fatal("runtime.discover omitted idempotency namespace")
	}
	second, _, err := OpenInstance(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second OpenInstance sharing data directory: %v", err)
	}
	secondResult := second.Endpoint().Invoke(t.Context(), "runtime.discover", struct{}{}, operation.Options{})
	if secondResult.Failure != nil {
		t.Fatalf("second runtime.discover: %v", secondResult.Failure)
	}
	secondDiscovery, ok := secondResult.Value.(*protocol.DiscoverResponse)
	if !ok || secondDiscovery.Capabilities.Limits.Idempotency.Namespace != namespace {
		t.Fatalf("second idempotency namespace = %+v, want %q", secondResult.Value, namespace)
	}
	if secondDiscovery.ServerInfo.InstanceID == discovery.ServerInfo.InstanceID {
		t.Fatalf("second Runtime instance identity = %q, want a fresh identity", secondDiscovery.ServerInfo.InstanceID)
	}
	if closeErr := second.Close(); closeErr != nil {
		t.Fatalf("close second instance: %v", closeErr)
	}

	if closeErr := instance.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	reopened, _, err := OpenInstance(t.Context(), cfg)
	if err != nil {
		t.Fatalf("reopen after Close: %v", err)
	}
	reopenedResult := reopened.Endpoint().Invoke(t.Context(), "runtime.discover", struct{}{}, operation.Options{})
	if reopenedResult.Failure != nil {
		t.Fatalf("runtime.discover after reopen: %v", reopenedResult.Failure)
	}
	reopenedDiscovery, ok := reopenedResult.Value.(*protocol.DiscoverResponse)
	if !ok || reopenedDiscovery.Capabilities.Limits.Idempotency.Namespace != namespace {
		t.Fatalf("reopened idempotency namespace = %+v, want %q", reopenedResult.Value, namespace)
	}
	if reopenedDiscovery.ServerInfo.InstanceID == discovery.ServerInfo.InstanceID ||
		reopenedDiscovery.ServerInfo.InstanceID == secondDiscovery.ServerInfo.InstanceID {
		t.Fatalf("reopened Runtime instance identity = %q, want a fresh identity", reopenedDiscovery.ServerInfo.InstanceID)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened instance: %v", err)
	}
}

func TestInstanceCloseIsIdempotent(t *testing.T) {
	done := make(chan struct{})
	close(done)
	instance := &Instance{
		delivery:      operationDelivery{service: &runtimeserver.Server{}},
		host:          &Host{},
		stopRuntime:   func() {},
		schedulerDone: done,
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}
}

func TestInstanceCloseRetainsResourcesUntilHostJoins(t *testing.T) {
	releaseComponent := make(chan struct{})
	resourceClosed := make(chan struct{})
	host := &Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		runCoordinator: shutdownFunc{wait: func(ctx context.Context) error {
			select {
			case <-releaseComponent:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}},
		hostResources: terminalClosers([]func() error{func() error {
			close(resourceClosed)
			return nil
		}}),
	}}
	done := make(chan struct{})
	close(done)
	instance := &Instance{
		delivery:        operationDelivery{service: &runtimeserver.Server{}},
		host:            host,
		stopRuntime:     func() {},
		schedulerDone:   done,
		shutdownTimeout: time.Millisecond,
	}
	if err := instance.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	close(releaseComponent)
	select {
	case <-resourceClosed:
	case <-time.After(time.Second):
		t.Fatal("Instance Host abandoned resources after the first caller timed out")
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("join completed Close: %v", err)
	}
}

type blockingInstanceOperationService struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
	startOne sync.Once
}

func (b *blockingInstanceOperationService) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	b.startOne.Do(func() { close(b.started) })
	<-ctx.Done()
	close(b.canceled)
	<-b.release
	return nil, ctx.Err()
}

func TestInstanceCloseJoinsAcceptedOperationsBeforeClosingResources(t *testing.T) {
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	service := &blockingInstanceOperationService{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	endpoint, err := operation.New(service, operation.Config{Lifetime: runtimeContext})
	if err != nil {
		t.Fatal(err)
	}
	schedulerDone := make(chan struct{})
	close(schedulerDone)
	resourceClosed := make(chan struct{})
	instance := &Instance{
		delivery: operationDelivery{endpoint: endpoint, service: &runtimeserver.Server{}},
		host: &Host{lifetime: &hostLifetime{hostResources: terminalClosers([]func() error{
			func() error {
				close(resourceClosed)
				return nil
			},
		})}},
		stopRuntime:   stopRuntime,
		schedulerDone: schedulerDone,
	}

	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		endpoint.Invoke(t.Context(), "runtime.discover", struct{}{}, operation.Options{})
	}()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- instance.Close() }()
	select {
	case <-service.canceled:
	case <-time.After(time.Second):
		t.Fatal("operation did not observe Close cancellation")
	}

	var resourceClosedEarly, closeReturnedEarly bool
	select {
	case <-resourceClosed:
		resourceClosedEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case <-closeDone:
		closeReturnedEarly = true
	default:
	}

	close(service.release)
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("released operation did not return")
	}
	if !resourceClosedEarly {
		select {
		case <-resourceClosed:
		case <-time.After(time.Second):
			t.Fatal("resource was not closed after the operation returned")
		}
	}
	if !closeReturnedEarly {
		select {
		case err := <-closeDone:
			if err != nil {
				t.Fatalf("Close: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Close did not return after the operation released")
		}
	}
	if resourceClosedEarly || closeReturnedEarly {
		t.Fatalf("Close crossed an active operation: resourceClosed=%t closeReturned=%t", resourceClosedEarly, closeReturnedEarly)
	}
}

func TestInstanceCloseContinuesGraphAfterCallerTimeout(t *testing.T) {
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	service := &blockingInstanceOperationService{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	endpoint, err := operation.New(service, operation.Config{Lifetime: runtimeContext})
	if err != nil {
		t.Fatal(err)
	}
	workersDone := make(chan struct{})
	close(workersDone)
	resourceClosed := make(chan struct{})
	instance := &Instance{
		delivery: operationDelivery{endpoint: endpoint, service: &runtimeserver.Server{}},
		host: &Host{lifetime: &hostLifetime{hostResources: terminalClosers([]func() error{
			func() error {
				close(resourceClosed)
				return nil
			},
		})}},
		stopRuntime:         stopRuntime,
		schedulerDone:       workersDone,
		databaseChangesDone: workersDone,
		recoveryDone:        workersDone,
		shutdownTimeout:     time.Millisecond,
	}

	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		endpoint.Invoke(t.Context(), "runtime.discover", struct{}{}, operation.Options{})
	}()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}

	if err := instance.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want caller deadline", err)
	}
	select {
	case <-service.canceled:
	case <-time.After(time.Second):
		t.Fatal("operation did not observe shutdown cancellation")
	}
	select {
	case <-resourceClosed:
		t.Fatal("Host resource closed before the accepted operation returned")
	default:
	}

	close(service.release)
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("released operation did not return")
	}
	select {
	case <-resourceClosed:
	case <-time.After(time.Second):
		t.Fatal("Instance abandoned its Host graph after caller timeout")
	}
}
