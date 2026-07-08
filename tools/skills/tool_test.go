package skills

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	skillsrc "github.com/Tangerg/lynx/skills"
)

func newToolFS() skillsrc.ResourceSource {
	return skillsrc.NewFS(fstest.MapFS{
		"pdf-processing/SKILL.md":                {Data: []byte("---\nname: pdf-processing\ndescription: Handle PDFs.\n---\n# PDF\nDo the thing. See references/REFERENCE.md.")},
		"pdf-processing/references/REFERENCE.md": {Data: []byte("detailed reference")},
		"data-analysis/SKILL.md":                 {Data: []byte("---\nname: data-analysis\ndescription: Analyze data.\n---\nanalysis body")},
	})
}

func newTool(t *testing.T) *Tool {
	t.Helper()
	tool, err := NewTool(newToolFS())
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	return tool
}

func TestNewToolNilSource(t *testing.T) {
	if _, err := NewTool(nil); !errors.Is(err, ErrNilSource) {
		t.Errorf("err = %v, want ErrNilSource", err)
	}
}

func TestCallList(t *testing.T) {
	out, err := newTool(t).Call(context.Background(), `{"op":"list"}`)
	if err != nil {
		t.Fatalf("Call list: %v", err)
	}
	if !strings.Contains(out, "<name>pdf-processing</name>") || !strings.Contains(out, "<name>data-analysis</name>") {
		t.Errorf("list output missing skills:\n%s", out)
	}
}

func TestCallLoad(t *testing.T) {
	out, err := newTool(t).Call(context.Background(), `{"op":"load","name":"pdf-processing"}`)
	if err != nil {
		t.Fatalf("Call load: %v", err)
	}
	if !strings.Contains(out, "Do the thing") {
		t.Errorf("load output missing instruction body:\n%s", out)
	}
}

func TestCallLoadResource(t *testing.T) {
	out, err := newTool(t).Call(context.Background(), `{"op":"load_resource","name":"pdf-processing","path":"references/REFERENCE.md"}`)
	if err != nil {
		t.Fatalf("Call load_resource: %v", err)
	}
	if out != "detailed reference" {
		t.Errorf("resource content = %q", out)
	}
}

func TestCallErrors(t *testing.T) {
	tool := newTool(t)
	cases := map[string]struct {
		args string
		want error
	}{
		"unknown op":       {`{"op":"frobnicate"}`, ErrUnknownOp},
		"load no name":     {`{"op":"load"}`, ErrNameRequired},
		"resource no path": {`{"op":"load_resource","name":"pdf-processing"}`, ErrPathRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := tool.Call(context.Background(), tc.args); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
