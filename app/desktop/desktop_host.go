package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
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

// The window whose frame the header is laid out around — narrowed to the one thing
// this file needs of it, so nothing here depends on the shape of a Wails window.
// `*application.WebviewWindow` satisfies it.
type nativeWindow interface {
	NativeWindow() unsafe.Pointer
}

// workingDirectoryPicker is owned by the host because choosing a directory is
// a packaged-application capability, not part of the Runtime Protocol. The Wails
// adapter satisfies it after the application and its window exist; tests use a
// small fake without constructing either.
type workingDirectoryPicker interface {
	ChooseWorkingDirectory() (string, error)
}

// imageSaver owns the native save dialog and the final write. The frontend only
// hands DesktopHost an inline image; it never receives a filesystem path and cannot
// bypass the platform picker with a browser download.
type imageSaver interface {
	SaveImage(suggestedFilename, mimeType string, contents []byte) (bool, error)
}

// DesktopHost is the Wails-owned boundary for capabilities that belong to the
// packaged application rather than the Runtime Protocol.
//
// EVERY EXPORTED METHOD ON THIS TYPE IS AN IPC ENTRY POINT. v3 binds a service by
// reflecting over its exported methods, so adding one here hands it to the frontend
// whether or not that was the intent — which is why the window is attached through an
// unexported setter rather than a method that reads like one. `TestDesktopHostBinds`
// pins the set.
type DesktopHost struct {
	localTokenPath         string
	pluginRoot             string
	workingDirectoryPicker workingDirectoryPicker
	imageSaver             imageSaver
	// Nil until the window exists. The application is constructed with this service
	// before it has any window, and `WindowChrome` is only reachable from a frontend
	// that a window had to load — but nil is answered honestly rather than assumed away.
	window nativeWindow
}

func newDesktopHost(home string) *DesktopHost {
	root := filepath.Join(home, ".lyra")
	return &DesktopHost{
		localTokenPath: filepath.Join(root, "local-token"),
		pluginRoot:     filepath.Join(root, "plugins"),
	}
}

// useWindow names the window whose chrome `WindowChrome` measures. Unexported on
// purpose: see the note on DesktopHost.
func (h *DesktopHost) useWindow(window nativeWindow) {
	h.window = window
}

// useWorkingDirectoryPicker attaches the packaged application's native directory
// chooser. Unexported on purpose: see the note on DesktopHost.
func (h *DesktopHost) useWorkingDirectoryPicker(picker workingDirectoryPicker) {
	h.workingDirectoryPicker = picker
}

// useImageSaver attaches the packaged application's native image-save capability.
// Unexported on purpose: see the note on DesktopHost.
func (h *DesktopHost) useImageSaver(saver imageSaver) {
	h.imageSaver = saver
}

func defaultDesktopHost() (*DesktopHost, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("desktop host: resolve user home: %w", err)
	}
	return newDesktopHost(home), nil
}

// WindowChrome hands the frontend the geometry of the platform's own window
// controls, so the header drawn under them can be laid out against measurements
// instead of literals.
//
// Read on demand rather than cached: the titlebar is rebuilt on the way into and
// out of fullscreen, and the controls go away entirely while fullscreen, so the
// answer has a shelf life of one layout.
func (h *DesktopHost) WindowChrome() WindowChrome {
	if h.window == nil {
		return WindowChrome{}
	}
	controlsCentreY, controlsInlineEnd, measured := nativeWindowChrome(h.window.NativeWindow())
	if !measured {
		return WindowChrome{}
	}
	return WindowChrome{
		ControlsCentreY:   controlsCentreY,
		ControlsInlineEnd: controlsInlineEnd,
		Measured:          true,
	}
}

// ChooseWorkingDirectory opens the platform directory picker and returns one
// absolute, existing directory. An empty string is the explicit cancellation
// result; it is not rewritten to the process working directory.
func (h *DesktopHost) ChooseWorkingDirectory() (string, error) {
	if h.workingDirectoryPicker == nil {
		return "", errors.New("desktop host: working directory picker is not configured")
	}
	selected, err := h.workingDirectoryPicker.ChooseWorkingDirectory()
	if err != nil {
		return "", fmt.Errorf("desktop host: choose working directory: %w", err)
	}
	if selected == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("desktop host: resolve working directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("desktop host: inspect working directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("desktop host: working directory %q is not a directory", absolute)
	}
	return filepath.Clean(absolute), nil
}

// SaveImage validates and decodes one inline image before handing it to the native
// save owner. The bool distinguishes a completed write from user cancellation.
func (h *DesktopHost) SaveImage(source string) (bool, error) {
	if h.imageSaver == nil {
		return false, errors.New("desktop host: image saver is not configured")
	}
	mimeType, extension, contents, err := decodeInlineImage(source)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	saved, err := h.imageSaver.SaveImage(suggestedImageFilename(extension), mimeType, contents)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	return saved, nil
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
