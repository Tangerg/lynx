package promptsource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainskills "github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const (
	governedSkillDocumentBytes = 1 << 20
	governedSkillsPerSource    = 256
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
	writeRuntimeSkill(t, root, "oversized", strings.Repeat("x", governedSkillDocumentBytes))

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
	for index := range governedSkillsPerSource + 1 {
		writeRuntimeSkill(t, root, fmt.Sprintf("skill-%03d", index), "instructions")
	}

	if _, err := ListSkills(t.Context(), root, ""); err == nil {
		t.Fatalf("ListSkills accepted more than %d entries from one source", governedSkillsPerSource)
	}
}
