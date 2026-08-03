package skill

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	skillspec "github.com/Tangerg/lynx/skills"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
)

type recordingProbe struct{ names []string }

func (p *recordingProbe) RecordUse(_ context.Context, name string, _ time.Time) error {
	p.names = append(p.names, name)
	return nil
}

type stubResourceSource struct{ loadErr error }

func (stubResourceSource) List(context.Context) ([]skillspec.Summary, error) { return nil, nil }

func (s stubResourceSource) Load(_ context.Context, name string) (*skillspec.Skill, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return &skillspec.Skill{Frontmatter: skillspec.Frontmatter{Name: name, Description: "d"}, Body: "b"}, nil
}

func (stubResourceSource) OpenResource(context.Context, string, string) (fs.File, error) {
	return nil, errors.New("not implemented")
}

func TestRecordingSourceRecordsOnSuccessfulLoad(t *testing.T) {
	probe := &recordingProbe{}
	src := recordingSource{ResourceSource: stubResourceSource{}, recorder: probe}
	if _, err := src.Load(context.Background(), "run-tests"); err != nil {
		t.Fatal(err)
	}
	if len(probe.names) != 1 || probe.names[0] != "run-tests" {
		t.Fatalf("recorded = %v, want [run-tests]", probe.names)
	}
}

func TestRecordingSourceSkipsUseOnLoadError(t *testing.T) {
	probe := &recordingProbe{}
	src := recordingSource{ResourceSource: stubResourceSource{loadErr: errors.New("absent")}, recorder: probe}
	if _, err := src.Load(context.Background(), "ghost"); err == nil {
		t.Fatal("expected a load error")
	}
	if len(probe.names) != 0 {
		t.Fatalf("recorded a use for a failed load: %v", probe.names)
	}
}

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

// TestBuild_MergesProjectOverGlobal proves the engine's skill tool
// layers <workdir>/.lyra/skills over the global dir, with the project copy
// winning on a name collision.
func TestBuild_MergesProjectOverGlobal(t *testing.T) {
	workdir := t.TempDir()
	global := t.TempDir()

	writeSkill(t, promptsource.ProjectSkillDir(workdir), "shared", "PROJECT copy")
	writeSkill(t, promptsource.ProjectSkillDir(workdir), "proj-only", "project only")
	writeSkill(t, global, "shared", "GLOBAL copy")
	writeSkill(t, global, "glob-only", "global only")

	tools, err := Build(workdir, global, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("Build returned %d tools, want 3", len(tools))
	}
	byName := make(map[string]toolcontract.Tool, len(tools))
	for _, candidate := range tools {
		byName[candidate.Definition().Name] = candidate
	}

	list, err := byName["list_skills"].Call(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"shared", "proj-only", "glob-only"} {
		if !strings.Contains(list, "<name>"+want+"</name>") {
			t.Errorf("list missing %q:\n%s", want, list)
		}
	}

	loaded, err := byName["load_skill"].Call(context.Background(), `{"name":"shared"}`)
	if err != nil {
		t.Fatalf("load shared: %v", err)
	}
	if !strings.Contains(loaded, "instructions for shared") {
		t.Errorf("load did not return the instruction body:\n%s", loaded)
	}
}

// TestBuild_AbsentWhenNoDirs proves the tool is omitted entirely when
// neither the project nor the global skills directory exists — no empty skill
// tool cluttering the model's tool list.
func TestBuild_AbsentWhenNoDirs(t *testing.T) {
	tools, err := Build(t.TempDir(), filepath.Join(t.TempDir(), "missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("Build returned %d tools without a skills directory", len(tools))
	}
}
