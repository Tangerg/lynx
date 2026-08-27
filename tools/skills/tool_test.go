package skills

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	toolcontract "github.com/Tangerg/scope/core/tool"
	skillsrc "github.com/Tangerg/scope/skills"
)

type panicSource struct{}

func (*panicSource) List(context.Context) ([]skillsrc.Summary, error) {
	panic("typed-nil source was used")
}

func (*panicSource) Load(context.Context, string) (*skillsrc.Skill, error) {
	panic("typed-nil source was used")
}

func (*panicSource) OpenResource(context.Context, string, string) (fs.File, error) {
	panic("typed-nil source was used")
}

func newToolFS() skillsrc.ResourceSource {
	repository, err := skillsrc.NewFS(fstest.MapFS{
		"pdf-processing/SKILL.md":                {Data: []byte("---\nname: pdf-processing\ndescription: Handle PDFs.\n---\n# PDF\nDo the thing. See references/REFERENCE.md.")},
		"pdf-processing/references/REFERENCE.md": {Data: []byte("detailed reference")},
		"data-analysis/SKILL.md":                 {Data: []byte("---\nname: data-analysis\ndescription: Analyze data.\n---\nanalysis body")},
	})
	if err != nil {
		panic(err)
	}
	return repository
}

func newTools(t *testing.T) map[string]toolcontract.Tool {
	t.Helper()
	built, err := NewTools(newToolFS())
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}
	tools := make(map[string]toolcontract.Tool, len(built))
	for _, candidate := range built {
		tools[candidate.Definition().Name] = candidate
	}
	return tools
}

func TestNewToolsRejectsNilSource(t *testing.T) {
	var typedNil *panicSource
	for _, source := range []skillsrc.ResourceSource{nil, typedNil} {
		if _, err := NewTools(source); !errors.Is(err, ErrNilSource) {
			t.Errorf("err = %v, want ErrNilSource", err)
		}
	}
}

func TestNewToolsBuildsOneStrictContractPerAction(t *testing.T) {
	tools := newTools(t)
	for _, name := range []string{"list_skills", "load_skill", "read_skill_resource"} {
		if tools[name] == nil {
			t.Errorf("missing %s: %v", name, tools)
		}
	}
	if len(tools) != 3 {
		t.Fatalf("tool names = %v, want exactly three", tools)
	}
	for name, candidate := range tools {
		if _, err := candidate.Call(t.Context(), `{"op":"list"}`); err == nil {
			t.Errorf("%s accepted removed op argument", name)
		}
	}
}

func TestListSkills(t *testing.T) {
	out, err := newTools(t)["list_skills"].Call(t.Context(), `{}`)
	if err != nil {
		t.Fatalf("list_skills: %v", err)
	}
	if !strings.Contains(out, "<name>pdf-processing</name>") || !strings.Contains(out, "<name>data-analysis</name>") {
		t.Errorf("list output missing skills:\n%s", out)
	}
}

func TestLoadSkill(t *testing.T) {
	out, err := newTools(t)["load_skill"].Call(t.Context(), `{"name":"pdf-processing"}`)
	if err != nil {
		t.Fatalf("load_skill: %v", err)
	}
	if !strings.Contains(out, "Do the thing") {
		t.Errorf("load output missing instruction body:\n%s", out)
	}
}

func TestReadSkillResource(t *testing.T) {
	out, err := newTools(t)["read_skill_resource"].Call(t.Context(), `{"name":"pdf-processing","path":"references/REFERENCE.md"}`)
	if err != nil {
		t.Fatalf("read_skill_resource: %v", err)
	}
	if out != "detailed reference" {
		t.Errorf("resource content = %q", out)
	}
}

func TestSkillToolsRejectMissingArguments(t *testing.T) {
	tools := newTools(t)
	for _, test := range []struct {
		name      string
		arguments string
	}{
		{"load_skill", `{}`},
		{"load_skill", `{"name":""}`},
		{"read_skill_resource", `{"name":"pdf-processing"}`},
		{"read_skill_resource", `{"name":"pdf-processing","path":""}`},
	} {
		if _, err := tools[test.name].Call(t.Context(), test.arguments); err == nil {
			t.Errorf("%s accepted %s", test.name, test.arguments)
		}
	}
}
