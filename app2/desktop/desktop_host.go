package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/desktop/remote"
	"github.com/Tangerg/lynx/app2/desktop/supervisor"
)

type DesktopBootstrap struct {
	Runtime supervisor.Connection `json:"runtime"`
}

// DesktopHost is the only Wails service allowed to expose Runtime supervision.
// Every exported method is an IPC entry point and is pinned by a binding test.
type DesktopHost struct {
	supervisor *supervisor.Supervisor
	remote     *remote.Manager
}

func newDesktopHost(runtimeSupervisor *supervisor.Supervisor, remoteManager *remote.Manager) (*DesktopHost, error) {
	if runtimeSupervisor == nil || remoteManager == nil {
		return nil, fmt.Errorf("desktop host: local and remote Runtime owners are required")
	}
	return &DesktopHost{supervisor: runtimeSupervisor, remote: remoteManager}, nil
}

func (host *DesktopHost) Bootstrap() (DesktopBootstrap, error) {
	if host.remote.Active() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		connection, err := host.remote.Bootstrap(ctx)
		if err != nil {
			return DesktopBootstrap{}, fmt.Errorf("desktop host: bootstrap remote Runtime: %w", err)
		}
		return DesktopBootstrap{Runtime: connection}, nil
	}
	connection, err := host.supervisor.Connection()
	if err != nil {
		return DesktopBootstrap{}, fmt.Errorf("desktop host: bootstrap Runtime: %w", err)
	}
	return DesktopBootstrap{Runtime: connection}, nil
}

func (host *DesktopHost) RemoteRuntime() remote.State { return host.remote.State() }

func (host *DesktopHost) ConnectRemoteRuntime(endpoint, token string) (remote.State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return host.remote.Configure(ctx, endpoint, token)
}

func (host *DesktopHost) UseLocalRuntime() (remote.State, error) {
	if err := host.remote.UseLocal(); err != nil {
		return remote.State{}, err
	}
	return host.remote.State(), nil
}

func (host *DesktopHost) UseRemoteRuntime() (remote.State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return host.remote.UseRemote(ctx)
}

func (host *DesktopHost) ForgetRemoteRuntime() (remote.State, error) {
	if err := host.remote.Forget(); err != nil {
		return remote.State{}, err
	}
	return host.remote.State(), nil
}
