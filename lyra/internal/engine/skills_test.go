package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, desc string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\ninstructions for " + name
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// TestBuildSkillTool_MergesProjectOverGlobal proves the engine's skill tool
// layers <workdir>/.lyra/skills over the global dir, with the project copy
// winning on a name collision.
func TestBuildSkillTool_MergesProjectOverGlobal(t *testing.T) {
	workdir := t.TempDir()
	global := t.TempDir()

	writeSkill(t, filepath.Join(workdir, projectSkillsSubdir), "shared", "PROJECT copy")
	writeSkill(t, filepath.Join(workdir, projectSkillsSubdir), "proj-only", "project only")
	writeSkill(t, global, "shared", "GLOBAL copy")
	writeSkill(t, global, "glob-only", "global only")

	tool := buildSkillTool(workdir, global)
	if tool == nil {
		t.Fatal("buildSkillTool returned nil despite existing skills dirs")
	}

	list, err := tool.Call(context.Background(), `{"op":"list"}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"shared", "proj-only", "glob-only"} {
		if !strings.Contains(list, "<name>"+want+"</name>") {
			t.Errorf("list missing %q:\n%s", want, list)
		}
	}

	loaded, err := tool.Call(context.Background(), `{"op":"load","name":"shared"}`)
	if err != nil {
		t.Fatalf("load shared: %v", err)
	}
	if !strings.Contains(loaded, "instructions for shared") {
		t.Errorf("load did not return the instruction body:\n%s", loaded)
	}
}

// TestBuildSkillTool_AbsentWhenNoDirs proves the tool is omitted entirely when
// neither the project nor the global skills directory exists — no empty skill
// tool cluttering the model's tool list.
func TestBuildSkillTool_AbsentWhenNoDirs(t *testing.T) {
	if tool := buildSkillTool(t.TempDir(), filepath.Join(t.TempDir(), "missing")); tool != nil {
		t.Error("buildSkillTool should return nil when no skills directory exists")
	}
}
