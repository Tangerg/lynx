package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mustDesktopHost(t *testing.T, home string) *DesktopHost {
	t.Helper()
	host, err := newDesktopHost(home)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

type workingDirectoryPickerFunc func() (string, error)

func (w workingDirectoryPickerFunc) ChooseWorkingDirectory() (string, error) {
	return w()
}

type imageSaverFunc func(suggestedFilename string, contents []byte) (bool, error)

func (i imageSaverFunc) SaveImage(suggestedFilename string, contents []byte) (bool, error) {
	return i(suggestedFilename, contents)
}

func TestDesktopHostBootstrap(t *testing.T) {
	home := t.TempDir()
	host := mustDesktopHost(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".scopeapp"), 0o700); err != nil {
		t.Fatal(err)
	}
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(filepath.Join(home, ".scopeapp", "local-token"), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.Endpoint != localRuntimeEndpoint || bootstrap.LocalRuntime.LocalToken != value {
		t.Fatalf("local runtime = %#v", bootstrap.LocalRuntime)
	}
}

func TestDesktopHostBootstrapRejectsInvalidDurableCredentials(t *testing.T) {
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	tests := map[string]struct {
		contents []byte
		mode     os.FileMode
		link     bool
	}{
		"arbitrary text":     {contents: []byte("token"), mode: 0o600},
		"whitespace wrapped": {contents: []byte("\n" + value + "\n"), mode: 0o600},
		"oversized":          {contents: bytes.Repeat([]byte("x"), 1<<20), mode: 0o600},
		"public permissions": {contents: []byte(value), mode: 0o644},
		"symbolic link":      {contents: []byte(value), mode: 0o600, link: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".scopeapp")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			tokenPath := filepath.Join(root, "local-token")
			writePath := tokenPath
			if test.link {
				writePath = filepath.Join(root, "actual-token")
			}
			if err := os.WriteFile(writePath, test.contents, test.mode); err != nil {
				t.Fatal(err)
			}
			if test.link {
				if err := os.Symlink(writePath, tokenPath); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink creation is unavailable: %v", err)
					}
					t.Fatal(err)
				}
			}

			if _, err := mustDesktopHost(t, home).Bootstrap(); err == nil {
				t.Fatal("Bootstrap() accepted an invalid durable credential")
			}
		})
	}
}

func TestDesktopHostBootstrapAllowsMissingState(t *testing.T) {
	bootstrap, err := mustDesktopHost(t, t.TempDir()).Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.LocalToken != "" {
		t.Fatalf("local token = %q, want empty", bootstrap.LocalRuntime.LocalToken)
	}
}

func TestDesktopHostChooseWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	host := mustDesktopHost(t, t.TempDir())
	host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
		return directory, nil
	}))

	selected, err := host.ChooseWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if selected != want {
		t.Fatalf("selected directory = %q, want %q", selected, want)
	}
}

func TestDesktopHostChooseWorkingDirectoryPreservesCancellation(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
		return "", nil
	}))

	selected, err := host.ChooseWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if selected != "" {
		t.Fatalf("canceled selection = %q, want empty", selected)
	}
}

func TestDesktopHostChooseWorkingDirectoryRequiresPicker(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())

	if selected, err := host.ChooseWorkingDirectory(); err == nil {
		t.Fatalf("ChooseWorkingDirectory() = %q, nil; want unconfigured picker error", selected)
	}
}

func TestDesktopHostChooseWorkingDirectoryRejectsInvalidSelections(t *testing.T) {
	tests := map[string]struct {
		selection func(t *testing.T) string
		pickErr   error
	}{
		"picker failure": {
			selection: func(t *testing.T) string { return "" },
			pickErr:   errors.New("dialog failed"),
		},
		"missing path": {
			selection: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
		},
		"regular file": {
			selection: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			host := mustDesktopHost(t, t.TempDir())
			host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
				return test.selection(t), test.pickErr
			}))

			if selected, err := host.ChooseWorkingDirectory(); err == nil {
				t.Fatalf("ChooseWorkingDirectory() = %q, nil; want error", selected)
			}
		})
	}
}

func TestDesktopHostSaveImageDecodesInlineMaterial(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	var filename string
	var contents []byte
	host.useImageSaver(imageSaverFunc(func(suggestedFilename string, material []byte) (bool, error) {
		filename = suggestedFilename
		contents = append([]byte(nil), material...)
		return true, nil
	}))

	saved, err := host.SaveImage("data:image/png;base64,aW1hZ2U=")
	if err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("SaveImage() = false, want completed save")
	}
	if !bytes.Equal(contents, []byte("image")) {
		t.Fatalf("saved material = %q; want image bytes", contents)
	}
	if !strings.HasPrefix(filename, "ScopeApp Image ") || !strings.HasSuffix(filename, ".png") {
		t.Fatalf("suggested filename = %q, want ScopeApp image PNG name", filename)
	}
}

func TestDesktopHostSaveImagePreservesCancellation(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
		return false, nil
	}))

	if saved, err := host.SaveImage("data:image/svg+xml,%3Csvg%3E%2B%3C%2Fsvg%3E"); err != nil {
		t.Fatal(err)
	} else if saved {
		t.Fatal("canceled SaveImage() = true, want false")
	}
}

func TestDesktopHostSaveImageRejectsInvalidSourcesBeforeOpeningDialog(t *testing.T) {
	tests := []string{
		"https://example.com/image.png",
		"data:text/plain;base64,aW1hZ2U=",
		"data:image/png;base64,%%broken%%",
		"data:image/png;base64,",
	}
	for _, source := range tests {
		t.Run(source, func(t *testing.T) {
			called := false
			host := mustDesktopHost(t, t.TempDir())
			host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
				called = true
				return true, nil
			}))

			if saved, err := host.SaveImage(source); err == nil {
				t.Fatalf("SaveImage(%q) = %v, nil; want validation error", source, saved)
			}
			if called {
				t.Fatal("native image saver opened for invalid source")
			}
		})
	}
}

func TestDesktopHostSaveImageRequiresAndPropagatesNativeOwner(t *testing.T) {
	if saved, err := mustDesktopHost(t, t.TempDir()).SaveImage("data:image/png;base64,aW1hZ2U="); err == nil {
		t.Fatalf("SaveImage() = %v, nil; want unconfigured saver error", saved)
	}

	host := mustDesktopHost(t, t.TempDir())
	host.useImageSaver(imageSaverFunc(func(string, []byte) (bool, error) {
		return false, errors.New("disk full")
	}))
	if saved, err := host.SaveImage("data:image/png;base64,aW1hZ2U="); err == nil {
		t.Fatalf("SaveImage() = %v, nil; want native save error", saved)
	}
}
