package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/Tangerg/lynx/app/runtime/localruntime"
)

const localRuntimeEndpoint = "http://127.0.0.1:17171"

type LocalRuntimeConnection struct {
	Endpoint   string `json:"endpoint"`
	LocalToken string `json:"localToken,omitempty"`
}

type DesktopBootstrap struct {
	LocalRuntime LocalRuntimeConnection `json:"localRuntime"`
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
	SaveImage(suggestedFilename string, contents []byte) (bool, error)
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
	}
}

// useWindow names the window whose chrome `WindowChrome` measures. Unexported on
// purpose: see the note on DesktopHost.
func (d *DesktopHost) useWindow(window nativeWindow) {
	d.window = window
}

// useWorkingDirectoryPicker attaches the packaged application's native directory
// chooser. Unexported on purpose: see the note on DesktopHost.
func (d *DesktopHost) useWorkingDirectoryPicker(picker workingDirectoryPicker) {
	d.workingDirectoryPicker = picker
}

// useImageSaver attaches the packaged application's native image-save capability.
// Unexported on purpose: see the note on DesktopHost.
func (d *DesktopHost) useImageSaver(saver imageSaver) {
	d.imageSaver = saver
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
func (d *DesktopHost) WindowChrome() WindowChrome {
	if d.window == nil {
		return WindowChrome{}
	}
	controlsCentreY, controlsInlineEnd, measured := nativeWindowChrome(d.window.NativeWindow())
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
func (d *DesktopHost) ChooseWorkingDirectory() (string, error) {
	if d.workingDirectoryPicker == nil {
		return "", errors.New("desktop host: working directory picker is not configured")
	}
	selected, err := d.workingDirectoryPicker.ChooseWorkingDirectory()
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
func (d *DesktopHost) SaveImage(source string) (bool, error) {
	if d.imageSaver == nil {
		return false, errors.New("desktop host: image saver is not configured")
	}
	extension, contents, err := decodeInlineImage(source)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	saved, err := d.imageSaver.SaveImage(suggestedImageFilename(extension), contents)
	if err != nil {
		return false, fmt.Errorf("desktop host: save image: %w", err)
	}
	return saved, nil
}

// Bootstrap returns the local runtime connection the frontend needs before it
// starts the application.
func (d *DesktopHost) Bootstrap() (DesktopBootstrap, error) {
	token, err := d.localToken()
	if err != nil {
		return DesktopBootstrap{}, err
	}
	return DesktopBootstrap{
		LocalRuntime: LocalRuntimeConnection{Endpoint: localRuntimeEndpoint, LocalToken: token},
	}, nil
}

func (d *DesktopHost) localToken() (string, error) {
	token, err := localruntime.ReadToken(d.localTokenPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("desktop host: read local runtime token: %w", err)
	}
	return token.Value(), nil
}
