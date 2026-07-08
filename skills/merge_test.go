package skills

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func skillFile(name, desc, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body)}
}

func TestMergePrecedence(t *testing.T) {
	project := NewFS(fstest.MapFS{
		"shared/SKILL.md":    skillFile("shared", "PROJECT copy", "project body"),
		"only-proj/SKILL.md": skillFile("only-proj", "project only", "x"),
	})
	global := NewFS(fstest.MapFS{
		"shared/SKILL.md":    skillFile("shared", "GLOBAL copy", "global body"),
		"only-glob/SKILL.md": skillFile("only-glob", "global only", "y"),
	})

	src := Merge(project, global) // project first → higher precedence

	list, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d summaries, want 3 (union, deduped): %v", len(list), list)
	}

	// The shared name must resolve to the project copy, not the global one.
	sk, err := src.Load(context.Background(), "shared")
	if err != nil {
		t.Fatalf("Load shared: %v", err)
	}
	if sk.Description != "PROJECT copy" {
		t.Errorf("shared description = %q, want the project copy (precedence)", sk.Description)
	}

	// A global-only skill is still reachable through the merge.
	if _, err := src.Load(context.Background(), "only-glob"); err != nil {
		t.Errorf("Load only-glob via merge: %v", err)
	}
}

func TestMergeSingleAndNil(t *testing.T) {
	only := NewFS(fstest.MapFS{})
	if got := Merge(only); got != only {
		t.Error("Merge of a single source should return it unchanged")
	}
	if got := Merge(nil, only, nil); got != only {
		t.Error("Merge should drop nils and unwrap to the lone source")
	}
}

// TestListMissingDir proves a source pointed at a non-existent directory lists
// empty rather than failing — the case behind a project/global skills dir that
// the user hasn't created.
func TestListMissingDir(t *testing.T) {
	got, err := Dir("/no/such/skills/dir").List(context.Background())
	if err != nil {
		t.Fatalf("List of a missing dir should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty list, got %v", got)
	}
}

// TestMergeReadResource proves resources are served from the first source
// that can satisfy it: the project copy of a shared skill wins, and a
// global-only skill's resource is still reachable through the merge.
func TestMergeReadResource(t *testing.T) {
	project := NewFS(fstest.MapFS{
		"shared/SKILL.md":           skillFile("shared", "project shared", "x"),
		"shared/references/note.md": {Data: []byte("PROJECT note")},
	})
	global := NewFS(fstest.MapFS{
		"shared/SKILL.md":           skillFile("shared", "global shared", "y"),
		"shared/references/note.md": {Data: []byte("GLOBAL note")},
		"glob-only/SKILL.md":        skillFile("glob-only", "global only", "z"),
		"glob-only/assets/data.txt": {Data: []byte("global asset")},
	})
	src := Merge(project, global)

	note, err := ReadResource(context.Background(), src, "shared", "references/note.md")
	if err != nil {
		t.Fatalf("ReadResource shared: %v", err)
	}
	if string(note) != "PROJECT note" {
		t.Errorf("shared resource = %q, want the project copy (precedence)", note)
	}

	asset, err := ReadResource(context.Background(), src, "glob-only", "assets/data.txt")
	if err != nil {
		t.Fatalf("ReadResource glob-only: %v", err)
	}
	if string(asset) != "global asset" {
		t.Errorf("glob-only resource = %q, want the global copy", asset)
	}
}

// TestMergeNoSources proves the degenerate empty merge is well-behaved: List
// is empty, and Load reports a clear not-found rather than a nil/nil result.
func TestMergeNoSources(t *testing.T) {
	src := Merge() // no sources

	if got, err := src.List(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("List on empty merge = (%v, %v), want (empty, nil)", got, err)
	}

	_, err := src.Load(context.Background(), "anything")
	if err == nil {
		t.Fatal("Load on empty merge should error")
	}
	if !strings.Contains(err.Error(), "no sources") {
		t.Errorf("err = %v, want a 'no sources' message", err)
	}
}
