package skills

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func skillFile(name, desc, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body)}
}

type cancelAfterListSource struct {
	ResourceSource
	cancel context.CancelFunc
}

func (s cancelAfterListSource) List(ctx context.Context) ([]Summary, error) {
	summaries, err := s.ResourceSource.List(ctx)
	s.cancel()
	return summaries, err
}

type cancelAfterLoadSource struct {
	ResourceSource
	cancel context.CancelFunc
}

func (s cancelAfterLoadSource) Load(ctx context.Context, name string) (*Skill, error) {
	skill, err := s.ResourceSource.Load(ctx, name)
	s.cancel()
	return skill, err
}

type cancelAfterOpenSource struct {
	ResourceSource
	cancel context.CancelFunc
}

func (s cancelAfterOpenSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	file, err := s.ResourceSource.OpenResource(ctx, name, resource)
	s.cancel()
	return file, err
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
	var typedNil *panicResourceSource
	if got := Merge(only); got != only {
		t.Error("Merge of a single source should return it unchanged")
	}
	if got := Merge(nil, typedNil, only, typedNil, nil); got != only {
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

func TestMergeKeepsResourcesWithWinningSkill(t *testing.T) {
	project := NewFS(fstest.MapFS{
		"shared/SKILL.md": skillFile("shared", "project shared", "project body without resource"),
	})
	global := NewFS(fstest.MapFS{
		"shared/SKILL.md":           skillFile("shared", "global shared", "global body"),
		"shared/references/note.md": {Data: []byte("GLOBAL note")},
	})

	_, err := ReadResource(t.Context(), Merge(project, global), "shared", "references/note.md")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadResource error = %v, want project resource not found", err)
	}
}

func TestMergeDoesNotMaskMalformedWinningSkill(t *testing.T) {
	project := NewFS(fstest.MapFS{
		"shared/SKILL.md": {Data: []byte("---\nname: shared\ndescription: \n---\nbroken")},
	})
	global := NewFS(fstest.MapFS{
		"shared/SKILL.md":           skillFile("shared", "global shared", "global body"),
		"shared/references/note.md": {Data: []byte("GLOBAL note")},
	})

	_, err := ReadResource(t.Context(), Merge(project, global), "shared", "references/note.md")
	if !errors.Is(err, ErrDescriptionEmpty) {
		t.Fatalf("ReadResource error = %v, want ErrDescriptionEmpty from project skill", err)
	}
}

func TestMergeTreatsEmptyMergedSourceAsNotFound(t *testing.T) {
	global := NewFS(fstest.MapFS{
		"global-skill/SKILL.md": skillFile("global-skill", "global skill", "body"),
	})

	skill, err := Merge(Merge(), global).Load(t.Context(), "global-skill")
	if err != nil {
		t.Fatalf("Load after empty merged source: %v", err)
	}
	if skill.Name != "global-skill" {
		t.Fatalf("loaded skill = %q, want global-skill", skill.Name)
	}
}

func TestMergeObservesCancellationAfterSourceCalls(t *testing.T) {
	base := NewFS(fstest.MapFS{
		"safe-skill/SKILL.md":           skillFile("safe-skill", "safe skill", "body"),
		"safe-skill/references/note.md": {Data: []byte("note")},
	})
	fallback := NewFS(fstest.MapFS{})
	tests := []struct {
		name   string
		source func(context.CancelFunc) ResourceSource
		call   func(context.Context, ResourceSource) error
	}{
		{
			name: "list",
			source: func(cancel context.CancelFunc) ResourceSource {
				return cancelAfterListSource{ResourceSource: base, cancel: cancel}
			},
			call: func(ctx context.Context, source ResourceSource) error {
				_, err := source.List(ctx)
				return err
			},
		},
		{
			name: "load",
			source: func(cancel context.CancelFunc) ResourceSource {
				return cancelAfterLoadSource{ResourceSource: base, cancel: cancel}
			},
			call: func(ctx context.Context, source ResourceSource) error {
				_, err := source.Load(ctx, "safe-skill")
				return err
			},
		},
		{
			name: "open resource",
			source: func(cancel context.CancelFunc) ResourceSource {
				return cancelAfterOpenSource{ResourceSource: base, cancel: cancel}
			},
			call: func(ctx context.Context, source ResourceSource) error {
				_, err := source.OpenResource(ctx, "safe-skill", "references/note.md")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			source := Merge(test.source(cancel), fallback)
			if err := test.call(ctx, source); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

// TestMergeNoSources proves the degenerate empty merge is well-behaved: List
// is empty, and Load reports the standard not-exist category rather than a
// nil/nil result or a string-only private error.
func TestMergeNoSources(t *testing.T) {
	src := Merge() // no sources

	if got, err := src.List(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("List on empty merge = (%v, %v), want (empty, nil)", got, err)
	}

	_, err := src.Load(context.Background(), "anything")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load error = %v, want fs.ErrNotExist", err)
	}
}
