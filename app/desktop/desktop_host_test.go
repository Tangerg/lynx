package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type workingDirectoryPickerFunc func() (string, error)

func (f workingDirectoryPickerFunc) ChooseWorkingDirectory() (string, error) {
	return f()
}

func TestDesktopHostBootstrap(t *testing.T) {
	home := t.TempDir()
	host := newDesktopHost(home)
	if err := os.MkdirAll(filepath.Join(home, ".lyra", "plugins", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lyra", "local-token"), []byte("token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".lyra", "plugins", "alpha", "index.js"), []byte("export default {};"), 0o600); err != nil {
		t.Fatal(err)
	}

	bootstrap, err := host.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.Endpoint != localRuntimeEndpoint || bootstrap.LocalRuntime.LocalToken != "token" {
		t.Fatalf("local runtime = %#v", bootstrap.LocalRuntime)
	}
	if len(bootstrap.SideloadedPlugins) != 1 || bootstrap.SideloadedPlugins[0].ID != "alpha" {
		t.Fatalf("plugins = %#v", bootstrap.SideloadedPlugins)
	}
}

func TestDesktopHostBootstrapAllowsMissingState(t *testing.T) {
	bootstrap, err := newDesktopHost(t.TempDir()).Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.LocalRuntime.LocalToken != "" {
		t.Fatalf("local token = %q, want empty", bootstrap.LocalRuntime.LocalToken)
	}
	if bootstrap.SideloadedPlugins == nil || bootstrap.SideloadIssues == nil {
		t.Fatalf("empty collections must encode as arrays: %#v", bootstrap)
	}
}

func TestDesktopHostChooseWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	host := newDesktopHost(t.TempDir())
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
	host := newDesktopHost(t.TempDir())
	host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
		return "", nil
	}))

	selected, err := host.ChooseWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if selected != "" {
		t.Fatalf("cancelled selection = %q, want empty", selected)
	}
}

func TestDesktopHostChooseWorkingDirectoryRequiresPicker(t *testing.T) {
	host := newDesktopHost(t.TempDir())

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
			host := newDesktopHost(t.TempDir())
			host.useWorkingDirectoryPicker(workingDirectoryPickerFunc(func() (string, error) {
				return test.selection(t), test.pickErr
			}))

			if selected, err := host.ChooseWorkingDirectory(); err == nil {
				t.Fatalf("ChooseWorkingDirectory() = %q, nil; want error", selected)
			}
		})
	}
}
