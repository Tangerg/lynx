package pathidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFollowsSymlinkBeforeMissingSuffix(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	got, err := Resolve(root, filepath.Join("alias", "new", "file.txt"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	physicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	want := filepath.Join(physicalTarget, "new", "file.txt")
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestContainsRequiresPhysicalTargets(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "src", "file.go")
	outside := filepath.Join(filepath.Dir(root), "outside", "file.go")

	for _, test := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "root", target: root, want: true},
		{name: "descendant", target: inside, want: true},
		{name: "sibling", target: outside, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Contains(root, test.target)
			if err != nil {
				t.Fatalf("Contains: %v", err)
			}
			if got != test.want {
				t.Fatalf("Contains(%q, %q) = %v, want %v", root, test.target, got, test.want)
			}
		})
	}
}
