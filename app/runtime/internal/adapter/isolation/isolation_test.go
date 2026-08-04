package isolation

import (
	"strings"
	"testing"
)

func TestNewRequiresExplicitAbsolutePaths(t *testing.T) {
	tests := []struct {
		name          string
		userHome      string
		baseDir       string
		readOnlyPaths []string
		want          string
	}{
		{name: "missing user home", baseDir: t.TempDir(), want: "user home is required"},
		{name: "relative user home", userHome: "home", baseDir: t.TempDir(), want: "user home must be absolute"},
		{name: "missing base directory", userHome: t.TempDir(), want: "base directory is required"},
		{name: "relative base directory", userHome: t.TempDir(), baseDir: "sandbox", want: "base directory must be absolute"},
		{name: "relative read-only path", userHome: t.TempDir(), baseDir: t.TempDir(), readOnlyPaths: []string{"cache"}, want: "read-only path 0 must be absolute"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolator, err := New(tt.userHome, tt.baseDir, tt.readOnlyPaths)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New error = %v, want containing %q", err, tt.want)
			}
			if isolator != nil {
				t.Fatalf("New isolator = %#v, want nil", isolator)
			}
		})
	}
}

func TestNewOwnsReadOnlyPaths(t *testing.T) {
	path := t.TempDir()
	paths := []string{path}
	isolator, err := New(t.TempDir(), t.TempDir(), paths)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	paths[0] = t.TempDir()
	if got := isolator.readOnlyPaths[0]; got != path {
		t.Fatalf("stored read-only path = %q, want owned value %q", got, path)
	}
}
