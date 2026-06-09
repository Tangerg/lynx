package skills

import (
	"context"
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
