package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// WindowChrome is where the platform put the window's own controls, in CSS pixels
// from the window's top-left.
//
// `Measured` false means there was nothing to measure — a platform whose controls
// sit outside the content, or no window yet — and the frontend keeps the gutter and
// alignment its stylesheet declares. Zeroes with `Measured` true are a different
// answer and a real one: the window is fullscreen, the marks are gone with the menu
// bar, and nothing should be reserved for or aligned to them.
type WindowChrome struct {
	ControlsCentreY   float64 `json:"controlsCentreY"`
	ControlsInlineEnd float64 `json:"controlsInlineEnd"`
	Measured          bool    `json:"measured"`
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
// It is also where the window's titlebar is set to the height that lines its own
// three controls up with the app's header. The controls themselves are the
// platform's — see window_chrome_darwin.go for what replacing them had cost.
func (h *DesktopHost) attachWindow(ctx context.Context) {
	h.window = ctx
	useCompactWindowToolbar()
}

// WindowChrome hands the frontend the geometry of the platform's own window
// controls, so the header drawn under them can be laid out against measurements
// instead of literals.
//
// Read on demand rather than cached: the titlebar is rebuilt on the way into and
// out of fullscreen, and the controls go away entirely while fullscreen, so the
// answer has a shelf life of one layout.
func (h *DesktopHost) WindowChrome() WindowChrome {
	controlsCentreY, controlsInlineEnd, measured := nativeWindowChrome()
	if !measured {
		return WindowChrome{}
	}
	return WindowChrome{
		ControlsCentreY:   controlsCentreY,
		ControlsInlineEnd: controlsInlineEnd,
		Measured:          true,
	}
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
