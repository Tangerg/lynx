package main

import (
	"fmt"

	"github.com/Tangerg/lynx/app2/desktop/supervisor"
)

type DesktopBootstrap struct {
	Runtime supervisor.Connection `json:"runtime"`
}

// DesktopHost is the only Wails service allowed to expose Runtime supervision.
// Every exported method is an IPC entry point and is pinned by a binding test.
type DesktopHost struct {
	supervisor *supervisor.Supervisor
}

func newDesktopHost(runtimeSupervisor *supervisor.Supervisor) (*DesktopHost, error) {
	if runtimeSupervisor == nil {
		return nil, fmt.Errorf("desktop host: Runtime supervisor is required")
	}
	return &DesktopHost{supervisor: runtimeSupervisor}, nil
}

func (host *DesktopHost) Bootstrap() (DesktopBootstrap, error) {
	connection, err := host.supervisor.Connection()
	if err != nil {
		return DesktopBootstrap{}, fmt.Errorf("desktop host: bootstrap Runtime: %w", err)
	}
	return DesktopBootstrap{Runtime: connection}, nil
}
