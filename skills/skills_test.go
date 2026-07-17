package skills

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

type panicFS struct{}

func (*panicFS) Open(string) (fs.File, error) {
	panic("typed-nil filesystem was used")
}

type panicResourceSource struct{}

func (*panicResourceSource) List(context.Context) ([]Summary, error) {
	panic("typed-nil source was used")
}

func (*panicResourceSource) Load(context.Context, string) (*Skill, error) {
	panic("typed-nil source was used")
}

func (*panicResourceSource) OpenResource(context.Context, string, string) (fs.File, error) {
	panic("typed-nil source was used")
}

type cancelingFS struct {
	fs.FS
	cancel context.CancelFunc
}

func (f cancelingFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	f.cancel()
	return file, err
}

type cancelingResourceSource struct {
	ResourceSource
	cancel context.CancelFunc
}

type failingOpenFS struct {
	fs.FS
	path string
	err  error
}

func (f failingOpenFS) Open(name string) (fs.File, error) {
	if name == f.path {
		return nil, f.err
	}
	return f.FS.Open(name)
}

func (s cancelingResourceSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	file, err := s.ResourceSource.OpenResource(ctx, name, resource)
	s.cancel()
	return file, err
}

type nilFileResourceSource struct{ ResourceSource }

func (s nilFileResourceSource) OpenResource(context.Context, string, string) (fs.File, error) {
	return nil, nil
}

const pdfSkill = `---
name: pdf-processing
description: Extract PDF text, fill forms, merge files. Use when handling PDFs.
license: Apache-2.0
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(git:*) Read
---
# PDF Processing

Step 1. Use the extract script.

See references/REFERENCE.md for details.
`

func newTestFS() ResourceSource {
	return NewFS(fstest.MapFS{
		"pdf-processing/SKILL.md":                {Data: []byte(pdfSkill)},
		"pdf-processing/references/REFERENCE.md": {Data: []byte("# Reference\nDetailed notes.")},
		"data-analysis/SKILL.md":                 {Data: []byte("---\nname: data-analysis\ndescription: Analyze data.\n---\nbody")},
		// A directory that is not a valid skill — must be skipped by List.
		"not-a-skill/readme.txt": {Data: []byte("ignore me")},
		"malformed/SKILL.md":     {Data: []byte("missing frontmatter")},
		"UPPER/SKILL.md":         skillFile("UPPER", "invalid directory name", "body"),
	})
}

func TestParse(t *testing.T) {
	fm, body, err := Parse([]byte(pdfSkill))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if fm.Name != "pdf-processing" {
		t.Errorf("name = %q", fm.Name)
	}
	if fm.License != "Apache-2.0" {
		t.Errorf("license = %q", fm.License)
	}
	if got := fm.Metadata["version"]; got != "1.0" {
		t.Errorf("metadata.version = %q", got)
	}
	if got := fm.AllowedToolList(); len(got) != 2 || got[0] != "Bash(git:*)" {
		t.Errorf("allowed tools = %v", got)
	}
	if body == "" || body[0] != '#' {
		t.Errorf("body should start with the markdown heading, got %q", body)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	for _, content := range []string{
		"no front matter here",
		" ---\nname: padded-open\ndescription: invalid fence\n---\nbody",
		"---\nname: padded-close\ndescription: invalid fence\n ---\nbody",
	} {
		if _, _, err := Parse([]byte(content)); !errors.Is(err, ErrNoFrontmatter) {
			t.Errorf("Parse(%q) error = %v, want ErrNoFrontmatter", content, err)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		fm   Frontmatter
		want error
	}{
		"ok":            {Frontmatter{Name: "pdf-tools", Description: "do things"}, nil},
		"empty name":    {Frontmatter{Description: "x"}, ErrNameEmpty},
		"upper name":    {Frontmatter{Name: "PDF", Description: "x"}, ErrNameInvalid},
		"padded name":   {Frontmatter{Name: " pdf", Description: "x"}, ErrNameInvalid},
		"double hyphen": {Frontmatter{Name: "a--b", Description: "x"}, ErrNameInvalid},
		"empty desc":    {Frontmatter{Name: "ok"}, ErrDescriptionEmpty},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.fm.Validate()
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSourceRejectsInvalidNamesBeforeFilesystemAccess(t *testing.T) {
	source := NewFS(&panicFS{})
	tests := []struct {
		name string
		want error
	}{
		{name: "", want: ErrNameEmpty},
		{name: "UPPER", want: ErrNameInvalid},
		{name: " padded", want: ErrNameInvalid},
		{name: "nested/skill", want: ErrNameInvalid},
		{name: strings.Repeat("a", maxNameLen+1), want: ErrNameTooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := source.Load(t.Context(), test.name); !errors.Is(err, test.want) {
				t.Fatalf("Load(%q) error = %v, want %v", test.name, err, test.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	got, err := newTestFS().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2 (non-skill dir skipped): %v", len(got), got)
	}
	// Sorted by name: data-analysis before pdf-processing.
	if got[0].Name != "data-analysis" || got[1].Name != "pdf-processing" {
		t.Errorf("summaries not sorted by name: %v", got)
	}
}

func TestLoad(t *testing.T) {
	sk, err := newTestFS().Load(context.Background(), "pdf-processing")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sk.Description == "" || sk.Body == "" {
		t.Errorf("loaded skill missing description or body: %+v", sk)
	}
}

func TestLoadClassifiesInvalidSkillAndPreservesCause(t *testing.T) {
	source := NewFS(fstest.MapFS{
		"bad-description/SKILL.md": {Data: []byte("---\nname: bad-description\ndescription:\n---\nbody")},
		"bad-document/SKILL.md":    {Data: []byte("missing frontmatter")},
		"mismatch/SKILL.md":        skillFile("another-name", "mismatched name", "body"),
	})

	for _, test := range []struct {
		name  string
		cause error
	}{
		{name: "bad-description", cause: ErrDescriptionEmpty},
		{name: "bad-document", cause: ErrNoFrontmatter},
		{name: "mismatch", cause: ErrNameMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := source.Load(t.Context(), test.name)
			if !errors.Is(err, ErrInvalidSkill) {
				t.Fatalf("Load error = %v, want ErrInvalidSkill", err)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("Load error = %v, want %v cause", err, test.cause)
			}
		})
	}

	_, err := source.Load(t.Context(), "missing")
	if !errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrInvalidSkill) {
		t.Fatalf("Load missing error = %v, want only fs.ErrNotExist", err)
	}
}

func TestListReturnsRepositoryReadFailure(t *testing.T) {
	readErr := errors.New("repository read failed")
	base := fstest.MapFS{
		"broken/SKILL.md": skillFile("broken", "broken skill", "body"),
	}
	source := NewFS(failingOpenFS{FS: base, path: "broken/SKILL.md", err: readErr})

	_, err := source.List(t.Context())
	if !errors.Is(err, readErr) {
		t.Fatalf("List error = %v, want repository read failure", err)
	}
}

func TestReadResource(t *testing.T) {
	fsrc := newTestFS()

	data, err := ReadResource(context.Background(), fsrc, "pdf-processing", "references/REFERENCE.md")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(data) == 0 {
		t.Error("resource content is empty")
	}

	// Traversal out of the skill directory must be rejected.
	if _, err := ReadResource(context.Background(), fsrc, "pdf-processing", "../data-analysis/SKILL.md"); !errors.Is(err, ErrResourcePath) {
		t.Errorf("traversal err = %v, want ErrResourcePath", err)
	}
}

func TestReadResourceRejectsNilFileWithoutPanicking(t *testing.T) {
	source := nilFileResourceSource{ResourceSource: newTestFS()}
	_, err := ReadResource(t.Context(), source, "pdf-processing", "references/REFERENCE.md")
	if !errors.Is(err, errNilResourceFile) {
		t.Fatalf("ReadResource error = %v, want errNilResourceFile", err)
	}
}

func TestResourcePathsArePortableAndRelative(t *testing.T) {
	source := NewFS(&panicFS{})
	for _, resource := range []string{
		"",
		".",
		"../other-skill/SKILL.md",
		"references//note.md",
		"references/../note.md",
		`references\..\sibling-skill\SKILL.md`,
		"/absolute/path",
	} {
		t.Run(resource, func(t *testing.T) {
			if _, err := source.OpenResource(t.Context(), "safe-skill", resource); !errors.Is(err, ErrResourcePath) {
				t.Fatalf("OpenResource(%q) error = %v, want ErrResourcePath", resource, err)
			}
		})
	}
}

func TestOperationsHonorCanceledContextBeforeAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	source := NewFS(&panicFS{})

	operations := []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error { _, err := source.List(ctx); return err }},
		{name: "load", call: func() error { _, err := source.Load(ctx, "safe-skill"); return err }},
		{name: "open resource", call: func() error {
			_, err := source.OpenResource(ctx, "safe-skill", "references/note.md")
			return err
		}},
		{name: "read resource", call: func() error {
			_, err := ReadResource(ctx, &panicResourceSource{}, "safe-skill", "references/note.md")
			return err
		}},
		{name: "empty merge list", call: func() error { _, err := Merge().List(ctx); return err }},
		{name: "empty merge load", call: func() error { _, err := Merge().Load(ctx, "safe-skill"); return err }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestOperationsHonorCancellationDuringAccess(t *testing.T) {
	base := fstest.MapFS{
		"safe-skill/SKILL.md":           skillFile("safe-skill", "safe skill", "body"),
		"safe-skill/references/note.md": {Data: []byte("note")},
	}
	for _, operation := range []struct {
		name string
		call func(context.Context, ResourceSource) error
	}{
		{name: "list", call: func(ctx context.Context, source ResourceSource) error {
			_, err := source.List(ctx)
			return err
		}},
		{name: "load", call: func(ctx context.Context, source ResourceSource) error {
			_, err := source.Load(ctx, "safe-skill")
			return err
		}},
		{name: "open resource", call: func(ctx context.Context, source ResourceSource) error {
			_, err := source.OpenResource(ctx, "safe-skill", "references/note.md")
			return err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			source := NewFS(cancelingFS{FS: base, cancel: cancel})
			if err := operation.call(ctx, source); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	source := cancelingResourceSource{ResourceSource: NewFS(base), cancel: cancel}
	if _, err := ReadResource(ctx, source, "safe-skill", "references/note.md"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadResource error = %v, want context.Canceled", err)
	}
}

func TestReadResourceRejectsNilSource(t *testing.T) {
	var typedNil *panicResourceSource
	tests := []struct {
		name   string
		source ResourceSource
	}{
		{name: "nil", source: nil},
		{name: "typed nil", source: typedNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ReadResource(t.Context(), test.source, "pdf-processing", "references/REFERENCE.md"); !errors.Is(err, errNilSource) {
				t.Fatalf("ReadResource error = %v, want errNilSource", err)
			}
		})
	}
}

func TestDirRejectsResourceSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "safe-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, SkillFile),
		[]byte("---\nname: safe-skill\ndescription: Safe skill.\n---\nbody"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("must not escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skillDir, "references", "secret.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	if _, err := ReadResource(t.Context(), Dir(root), "safe-skill", "references/secret.txt"); err == nil {
		t.Fatal("ReadResource followed a symlink outside the source root")
	}
}

func TestDirRejectsResourceSymlinkEscapingSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "safe-skill")
	otherDir := filepath.Join(root, "other-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, SkillFile),
		[]byte("---\nname: safe-skill\ndescription: Safe skill.\n---\nbody"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "secret.txt"), []byte("sibling secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skillDir, "references", "secret.txt")
	if err := os.Symlink(filepath.Join("..", "..", "other-skill", "secret.txt"), link); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	if _, err := ReadResource(t.Context(), Dir(root), "safe-skill", "references/secret.txt"); err == nil {
		t.Fatal("ReadResource followed a symlink into a sibling skill")
	}
}

func TestNewFSRejectsNilFilesystemWithoutPanicking(t *testing.T) {
	var typedNil *panicFS
	tests := []struct {
		name       string
		filesystem fs.FS
	}{
		{name: "nil", filesystem: nil},
		{name: "typed nil", filesystem: typedNil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewFS(test.filesystem).List(t.Context()); !errors.Is(err, errNilFS) {
				t.Fatalf("List error = %v, want errNilFS", err)
			}
		})
	}
}
