package server

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/workspacepath"
)

func canonicalWorkspacePath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := workspacepath.Canonical(path)
	if err != nil {
		t.Fatalf("canonical workspace path %q: %v", path, err)
	}
	return canonical
}
