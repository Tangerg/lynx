package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIgnoreMatch audits the gitignore subset the watcher relies on: basename
// at any depth, root anchoring, directory-only, globs, and negation last-wins.
func TestIgnoreMatch(t *testing.T) {
	ig := &Ignore{}
	for _, line := range []string{
		"# comment",
		"",
		"node_modules",   // basename, any depth
		"*.log",          // glob, any depth
		"/build",         // root-anchored
		"dist/",          // directory-only
		"coverage/**",    // everything under
		"keep.log",       // (then re-included below)
		"!important.log", // negation re-includes
	} {
		if r, ok := compileIgnoreRule(line); ok {
			ig.rules = append(ig.rules, r)
		}
	}

	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},          // basename at root
		{"pkg/sub/node_modules", true, true},  // basename deep
		{"src/app.log", false, true},          // *.log deep
		{"build", true, true},                 // /build anchored at root
		{"src/build", true, false},            // /build NOT matched deeper
		{"dist", true, true},                  // dist/ dir-only, is a dir
		{"dist", false, false},                // dist/ must not match a file named dist
		{"coverage/unit/x.html", false, true}, // coverage/** under
		{"important.log", false, false},       // *.log then !important.log → re-included
		{"src/main.go", false, false},         // not ignored
	}
	for _, c := range cases {
		if got := ig.Match(c.path, c.isDir); got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}

// TestLoadIgnore reads a real .gitignore + .git/info/exclude off disk and
// merges both sources.
func TestLoadIgnore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte("secret/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ig := LoadIgnore(root)
	if !ig.Match("node_modules", true) {
		t.Error("want node_modules ignored (from .gitignore)")
	}
	if !ig.Match("a/b.tmp", false) {
		t.Error("want *.tmp ignored (from .gitignore)")
	}
	if !ig.Match("secret", true) {
		t.Error("want secret/ ignored (from .git/info/exclude)")
	}
	if ig.Match("src/main.go", false) {
		t.Error("src/main.go must not be ignored")
	}

	// A root with no ignore files yields an empty, harmless matcher.
	if LoadIgnore(t.TempDir()).Match("anything", true) {
		t.Error("empty matcher must ignore nothing")
	}
}
