package bootstrap

import (
	"context"
	"errors"
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
		ServerInfo:           protocol.ServerInfo{Name: "test-runtime", Version: "test-version"},
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
	if _, _, err := OpenInstance(t.Context(), cfg); !errors.Is(err, ErrDataDirectoryInUse) {
		t.Fatalf("second OpenInstance error = %v, want ErrDataDirectoryInUse", err)
	}

	if err := instance.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, _, err := OpenInstance(t.Context(), cfg)
	if err != nil {
		t.Fatalf("reopen after Close: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened instance: %v", err)
	}
}

func TestInstanceCloseReleasesDataDirectoryLastAndIsIdempotent(t *testing.T) {
	directory := t.TempDir()
	lease, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	done := make(chan struct{})
	close(done)
	instance := &Instance{
		service:       &runtimeserver.Server{},
		host:          &Host{},
		lease:         lease,
		stopRuntime:   func() {},
		schedulerDone: done,
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}

	next, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire after Close: %v", err)
	}
	if err := next.release(); err != nil {
		t.Fatalf("release next lease: %v", err)
	}
}

func TestInstanceCloseRetainsLeaseUntilHostJoins(t *testing.T) {
	directory := t.TempDir()
	lease, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	ready := false
	host := &Host{lifetime: &hostLifetime{
		shutdownTimeout: time.Millisecond,
		runCoordinator: shutdownFunc{wait: func(ctx context.Context) error {
			if ready {
				return nil
			}
			<-ctx.Done()
			return ctx.Err()
		}},
	}}
	done := make(chan struct{})
	close(done)
	instance := &Instance{
		service:       &runtimeserver.Server{},
		host:          host,
		lease:         lease,
		stopRuntime:   func() {},
		schedulerDone: done,
	}
	if err := instance.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	if _, err := acquireDataDirectoryLease(directory); !errors.Is(err, ErrDataDirectoryInUse) {
		t.Fatalf("lease after incomplete Close = %v, want ErrDataDirectoryInUse", err)
	}

	ready = true
	if err := instance.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	next, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire after retry Close: %v", err)
	}
	if err := next.release(); err != nil {
		t.Fatalf("release next lease: %v", err)
	}
}
