package promptsource

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/Tangerg/lynx/skills"

	domainskills "github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func writeRuntimeSkill(t *testing.T, root, name, body string) {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create skill %q: %v", name, err)
	}
	document := fmt.Sprintf("---\nname: %s\ndescription: A valid Runtime skill used by the bounded-source counterexample.\n---\n%s", name, body)
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatalf("write skill %q: %v", name, err)
	}
}

func TestRuntimeSkillSourceRejectsOversizedDocument(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "oversized", strings.Repeat("x", domainskills.MaxAuthoredSkillDocumentBytes))

	source := MergeSkillSource("", root, nil)
	if source == nil {
		t.Fatal("configured source is nil")
	}
	if _, err := source.Load(t.Context(), "oversized"); !errors.Is(err, domainskills.ErrDocumentTooLarge) {
		t.Fatalf("Load error = %v, want ErrDocumentTooLarge before the document is materialized", err)
	}
}

func TestRuntimeSkillSourceRejectsOverCapacityDirectory(t *testing.T) {
	root := t.TempDir()
	for index := range domainskills.MaxSkillsPerSource + 1 {
		writeRuntimeSkill(t, root, fmt.Sprintf("skill-%03d", index), "instructions")
	}

	if _, err := ListSkills(t.Context(), root, ""); !errors.Is(err, domainskills.ErrLibraryCapacity) {
		t.Fatalf("ListSkills error = %v, want ErrLibraryCapacity beyond %d entries", err, domainskills.MaxSkillsPerSource)
	}
}

func TestRuntimeSkillSourceRejectsRawDirectoryFlood(t *testing.T) {
	root := t.TempDir()
	for index := range domainskills.MaxSkillDirectoryEntries + 1 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("junk-%03d", index)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ListSkills(t.Context(), root, ""); !errors.Is(err, domainskills.ErrLibraryCapacity) {
		t.Fatalf("ListSkills raw-directory error = %v, want ErrLibraryCapacity", err)
	}
}

func TestRuntimeSkillSourceRejectsOversizedResource(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "with-resource", "Read references/large.txt")
	resourceDir := filepath.Join(root, "with-resource", "references")
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(resourceDir, "large.txt"),
		[]byte(strings.Repeat("x", domainskills.MaxSkillResourceBytes+1)),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	source := MergeSkillSource("", root, nil)
	if _, err := sdk.ReadResource(t.Context(), source, "with-resource", "references/large.txt"); !errors.Is(err, domainskills.ErrResourceTooLarge) {
		t.Fatalf("ReadResource error = %v, want ErrResourceTooLarge", err)
	}
}

func TestRuntimeSkillSourceRejectsResourceGrowthAfterOpen(t *testing.T) {
	root := t.TempDir()
	writeRuntimeSkill(t, root, "growing-resource", "Read references/growing.txt")
	resourceDir := filepath.Join(root, "growing-resource", "references")
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resourceDir, "growing.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", domainskills.MaxSkillResourceBytes)), 0o644); err != nil {
		t.Fatal(err)
	}

	source := MergeSkillSource("", root, nil)
	file, err := source.OpenResource(t.Context(), "growing-resource", "references/growing.txt")
	if err != nil {
		t.Fatalf("OpenResource at exact limit: %v", err)
	}
	if appendFile, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	} else if _, err := appendFile.WriteString("x"); err != nil {
		_ = appendFile.Close()
		_ = file.Close()
		t.Fatal(err)
	} else if err := appendFile.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if !errors.Is(errors.Join(readErr, closeErr), domainskills.ErrResourceTooLarge) {
		t.Fatalf("grown resource error = %v, want ErrResourceTooLarge", errors.Join(readErr, closeErr))
	}
}
