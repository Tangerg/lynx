package workspacepath_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func TestCanonicalNormalizesSpellingsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	want := workspacepath.Canonical(dir)
	for _, spelling := range []string{
		dir + string(filepath.Separator),
		filepath.Join(dir, "."),
		filepath.Join(dir, "sub", ".."),
	} {
		if got := workspacepath.Canonical(spelling); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", spelling, got, want)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if got := workspacepath.Canonical(link); got != want {
		t.Fatalf("Canonical(symlink) = %q, want %q", got, want)
	}
	if got := workspacepath.Canonical(""); got != "" {
		t.Fatalf("Canonical(empty) = %q", got)
	}
}

func TestResolverConfinesWorkspacePaths(t *testing.T) {
	resolver := workspacepath.Resolver{}
	root := t.TempDir()
	for path, want := range map[string]string{
		"main.go":                       "main.go",
		"pkg/util.go":                   "pkg/util.go",
		"./a/../b.go":                   "b.go",
		filepath.Join(root, "sub/x.go"): "sub/x.go",
	} {
		got, err := resolver.ResolveInRoot(root, path)
		if err != nil || got != want {
			t.Errorf("ResolveInRoot(%q) = (%q, %v), want (%q, nil)", path, got, err, want)
		}
	}
	for _, path := range []string{"../escape.go", "../../etc/passwd", "/etc/passwd", "sub/../../out.go"} {
		if _, err := resolver.ResolveInRoot(root, path); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
			t.Errorf("ResolveInRoot(%q) error = %v, want ErrPathOutsideRoot", path, err)
		}
	}
	if _, err := resolver.ResolveInRoot(root, ""); !errors.Is(err, workspaceapp.ErrPathRequired) {
		t.Fatalf("ResolveInRoot empty error = %v, want ErrPathRequired", err)
	}
}

func TestResolverRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape.txt")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := (workspacepath.Resolver{}).ResolveExistingInRoot(root, "escape.txt"); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("ResolveExistingInRoot escape error = %v, want ErrPathOutsideRoot", err)
	}
}

func TestResolveExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := workspacepath.ResolveExistingDir(filepath.Join(dir, "."))
	if err != nil || got != workspacepath.Canonical(dir) {
		t.Fatalf("ResolveExistingDir = %q, %v", got, err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := workspacepath.ResolveExistingDir(file); !errors.Is(err, workspacepath.ErrNotDirectory) {
		t.Fatalf("file error = %v, want ErrNotDirectory", err)
	}
	if _, err := workspacepath.ResolveExistingDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory must fail")
	}
}

func TestResolverInspectFindsRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	identity, err := (workspacepath.Resolver{}).Inspect(nested)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if identity.Cwd != workspacepath.Canonical(nested) || identity.ProjectRoot != workspacepath.Canonical(root) || identity.Missing {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestResolverInspectReportsUnavailableWorkspace(t *testing.T) {
	empty, err := (workspacepath.Resolver{}).Inspect("")
	if err != nil || !empty.Missing || empty.Cwd != "" || empty.ProjectRoot != "" {
		t.Fatalf("Inspect empty = (%+v, %v), want unavailable empty identity", empty, err)
	}

	missing := filepath.Join(t.TempDir(), "gone")
	identity, err := (workspacepath.Resolver{}).Inspect(missing)
	if err != nil {
		t.Fatalf("Inspect missing: %v", err)
	}
	if !identity.Missing || identity.Cwd != workspacepath.Canonical(missing) || identity.ProjectRoot != identity.Cwd {
		t.Fatalf("missing identity = %+v", identity)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	identity, err = (workspacepath.Resolver{}).Inspect(file)
	if err != nil || !identity.Missing {
		t.Fatalf("Inspect file = (%+v, %v), want unavailable", identity, err)
	}
}
