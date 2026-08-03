package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const localRuntimeEndpoint = "http://127.0.0.1:17171"

type LocalRuntimeConnection struct {
	Endpoint   string `json:"endpoint"`
	LocalToken string `json:"localToken,omitempty"`
}

type SideloadedPlugin struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type SideloadIssue struct {
	ID     string `json:"id"`
	Detail string `json:"detail"`
}

type DesktopBootstrap struct {
	LocalRuntime      LocalRuntimeConnection `json:"localRuntime"`
	SideloadedPlugins []SideloadedPlugin     `json:"sideloadedPlugins"`
	SideloadIssues    []SideloadIssue        `json:"sideloadIssues"`
}

// DesktopHost is the Wails-owned boundary for capabilities that belong to the
// packaged application rather than the Runtime Protocol.
type DesktopHost struct {
	localTokenPath string
	pluginRoot     string
	// The Wails application context, handed over at startup. Held as a field
	// rather than threaded per call because it is the window's identity, not a
	// request's: the runtime's window API takes it, and the frontend's calls
	// arrive over the binding with no context of their own.
	window context.Context
}

func newDesktopHost(home string) *DesktopHost {
	root := filepath.Join(home, ".lyra")
	return &DesktopHost{
		localTokenPath: filepath.Join(root, "local-token"),
		pluginRoot:     filepath.Join(root, "plugins"),
	}
}

func defaultDesktopHost() (*DesktopHost, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve user home: %w", err)
	}
	return newDesktopHost(home), nil
}

// attachWindow receives the Wails application context at startup. Everything
// below that drives the window is inert until it has been called.
//
// It is also where the app takes the window's controls over from the platform:
// the frame's own buttons go away here, and from this point the three the app
// draws are the only ones.
func (h *DesktopHost) attachWindow(ctx context.Context) {
	h.window = ctx
	hideNativeWindowButtons()
}

// MinimiseWindow, ToggleMaximiseWindow and CloseWindow back the three controls
// the app draws itself, since the platform no longer draws any. They are
// no-ops before startup and in tests, where there is no window to act on.
func (h *DesktopHost) MinimiseWindow() {
	if h.window == nil {
		return
	}
	runtime.WindowMinimise(h.window)
}

func (h *DesktopHost) ToggleMaximiseWindow() {
	if h.window == nil {
		return
	}
	runtime.WindowToggleMaximise(h.window)
}

// CloseWindow ends the application. On macOS a single-window app with no dock
// presence to return to has nothing left to show once its window is gone, so
// the red control quits rather than orphaning a running process.
func (h *DesktopHost) CloseWindow() {
	if h.window == nil {
		return
	}
	runtime.Quit(h.window)
}

// IsWindowMaximised answers which way the zoom control points. The platform
// draws two different marks — arrows out to fill the screen, arrows in to come
// back — and a control that shows the same one in both states is telling you
// what it does half the time.
func (h *DesktopHost) IsWindowMaximised() bool {
	if h.window == nil {
		return false
	}
	return runtime.WindowIsMaximised(h.window)
}

// Bootstrap returns the local runtime connection and immutable plugin sources
// the frontend needs before it starts loading application plugins.
func (h *DesktopHost) Bootstrap() (DesktopBootstrap, error) {
	token, err := h.localToken()
	if err != nil {
		return DesktopBootstrap{}, err
	}
	plugins, issues, err := h.sideloadedPlugins()
	if err != nil {
		return DesktopBootstrap{}, err
	}
	return DesktopBootstrap{
		LocalRuntime:      LocalRuntimeConnection{Endpoint: localRuntimeEndpoint, LocalToken: token},
		SideloadedPlugins: plugins,
		SideloadIssues:    issues,
	}, nil
}

func (h *DesktopHost) localToken() (string, error) {
	data, err := os.ReadFile(h.localTokenPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("desktop host: read local runtime token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (h *DesktopHost) sideloadedPlugins() ([]SideloadedPlugin, []SideloadIssue, error) {
	entries, err := os.ReadDir(h.pluginRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []SideloadedPlugin{}, []SideloadIssue{}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("desktop host: read plugin directory: %w", err)
	}

	plugins := make([]SideloadedPlugin, 0, len(entries))
	issues := make([]SideloadIssue, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		source, readErr := os.ReadFile(filepath.Join(h.pluginRoot, id, "index.js"))
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			issues = append(issues, SideloadIssue{ID: id, Detail: readErr.Error()})
			continue
		}
		if len(source) == 0 {
			issues = append(issues, SideloadIssue{ID: id, Detail: "index.js is empty"})
			continue
		}
		plugins = append(plugins, SideloadedPlugin{ID: id, Source: string(source)})
	}
	return plugins, issues, nil
}
