package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

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
	if _, _, err := Parse([]byte("no front matter here")); !errors.Is(err, ErrNoFrontmatter) {
		t.Errorf("err = %v, want ErrNoFrontmatter", err)
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

func TestReadResourceRejectsNilSource(t *testing.T) {
	if _, err := ReadResource(context.Background(), nil, "pdf-processing", "references/REFERENCE.md"); err == nil {
		t.Fatal("nil source must error")
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

func TestNewFSRejectsNilFilesystemWithoutPanicking(t *testing.T) {
	if _, err := NewFS(nil).List(t.Context()); !errors.Is(err, errNilFS) {
		t.Fatalf("List error = %v, want errNilFS", err)
	}
}
