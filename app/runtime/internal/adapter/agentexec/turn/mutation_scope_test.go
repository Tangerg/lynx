package turn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools/fs"
)

func TestFileMutationScopeUsesConcreteApplyPatchSchema(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	patch := "--- a/" + outside + "\n+++ b/" + outside + "\n@@ -0,0 +1 @@\n+outside\n"
	arguments, err := json.Marshal(fs.ApplyPatchRequest{Patch: patch})
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}

	got := fileMutationScope(fs.NewApplyPatchTool(nil), string(arguments), workspace)
	if got != tool.FileMutationOutsideWorkspace {
		t.Fatalf("fileMutationScope() = %v, want outside workspace", got)
	}
}

func TestFileMutationScopeResolvesSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "alias")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	reporter := mutationReporter{paths: []string{filepath.Join("alias", "file.txt")}}

	got := fileMutationScope(reporter, `{}`, workspace)
	if got != tool.FileMutationOutsideWorkspace {
		t.Fatalf("fileMutationScope() = %v, want outside workspace", got)
	}
}

func TestFileMutationScopeFailsClosedWhenArgumentsCannotBeInspected(t *testing.T) {
	got := fileMutationScope(fs.NewApplyPatchTool(nil), `{not json`, t.TempDir())
	if got != tool.FileMutationUnknown {
		t.Fatalf("fileMutationScope() = %v, want unknown", got)
	}
}

type mutationReporter struct {
	paths []string
}

func (r mutationReporter) MutationPaths(string) ([]string, error) {
	return r.paths, nil
}
