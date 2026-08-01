package text

import (
	"strings"
	"testing"
)

type greetingData struct {
	Name string
	Age  int
}

func TestNewRenderer(t *testing.T) {
	renderer := NewRenderer(struct{}{})
	got, err := renderer.Render()
	if err != nil || got != "" {
		t.Errorf("empty Render = (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestRendererTypedData(t *testing.T) {
	renderer := NewRenderer(greetingData{Name: "world"}).SetTemplate("Hello {{.Name}}")
	got, err := renderer.Render()
	if err != nil || got != "Hello world" {
		t.Errorf("Render = %q, %v", got, err)
	}
}

func TestRendererSetDataReusesTemplate(t *testing.T) {
	renderer := NewRenderer(greetingData{Name: "first"}).SetTemplate("Hi {{.Name}}")
	if _, err := renderer.Render(); err != nil {
		t.Fatal(err)
	}
	renderer.SetData(greetingData{Name: "second"})
	got, err := renderer.Render()
	if err != nil || got != "Hi second" {
		t.Fatalf("Render after SetData = %q, %v", got, err)
	}
}

func TestRendererDoesNotCacheRenderedOutput(t *testing.T) {
	data := map[string]string{"name": "first"}
	renderer := NewRenderer(data).SetTemplate("Hi {{.name}}")
	if _, err := renderer.Render(); err != nil {
		t.Fatal(err)
	}
	data["name"] = "second"
	got, err := renderer.Render()
	if err != nil || got != "Hi second" {
		t.Fatalf("Render after referenced data mutation = %q, %v", got, err)
	}
}

func TestRendererSetDelimiters(t *testing.T) {
	got, err := NewRenderer(greetingData{Name: "alice"}).
		SetDelimiters("[[", "]]").
		SetTemplate("Hello [[.Name]]").
		Render()
	if err != nil || got != "Hello alice" {
		t.Errorf("Render = %q, %v", got, err)
	}
}

func TestRendererEmptyDelimitersRestoreDefaults(t *testing.T) {
	got, err := NewRenderer(greetingData{Name: "alice"}).
		SetDelimiters("", "").
		SetTemplate("Hello {{.Name}}").
		Render()
	if err != nil || got != "Hello alice" {
		t.Errorf("Render = %q, %v", got, err)
	}
}

func TestRendererReset(t *testing.T) {
	renderer := NewRenderer(greetingData{Name: "before"}).SetTemplate("{{.Name}}")
	renderer.Reset(greetingData{Name: "after"})
	got, err := renderer.Render()
	if err != nil || got != "" {
		t.Errorf("Render after Reset = %q, %v", got, err)
	}
}

func TestRendererClone(t *testing.T) {
	original := NewRenderer(greetingData{Name: "alice"}).SetTemplate("Hi {{.Name}}")
	clone := original.Clone()
	original.SetData(greetingData{Name: "bob"})
	got, err := clone.Render()
	if err != nil || got != "Hi alice" {
		t.Errorf("clone Render = %q, %v", got, err)
	}
}

func TestRendererRenderErrors(t *testing.T) {
	if _, err := NewRenderer(struct{}{}).SetTemplate("{{.bad").Render(); err == nil || !strings.Contains(err.Error(), "parse template") {
		t.Fatalf("parse error = %v", err)
	}
	if _, err := NewRenderer(map[string]string{}).SetTemplate("{{.missing}}").Render(); err == nil || !strings.Contains(err.Error(), "execute template") {
		t.Fatalf("missing-key error = %v", err)
	}
}

func TestRendererMustRenderPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	NewRenderer(struct{}{}).SetTemplate("{{.bad").MustRender()
}

func TestRendererRequireVariables(t *testing.T) {
	renderer := NewRenderer(greetingData{}).SetTemplate("Hi {{.Name}}, you are {{.Age}}")
	if err := renderer.RequireVariables("Name", "Age"); err != nil {
		t.Errorf("RequireVariables = %v", err)
	}
	if err := renderer.RequireVariables("Name", "Missing"); err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Errorf("missing variable error = %v", err)
	}
	if err := renderer.RequireVariables(""); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("invalid variable error = %v", err)
	}
}

func TestRendererRequireVariablesCustomDelimiters(t *testing.T) {
	renderer := NewRenderer(greetingData{}).
		SetDelimiters("[[", "]]").
		SetTemplate("Hi [[.Name]]")
	if err := renderer.RequireVariables("Name"); err != nil {
		t.Errorf("RequireVariables = %v", err)
	}
}

func TestPackageRender(t *testing.T) {
	got, err := Render("Hello {{.Name}}", greetingData{Name: "alice"})
	if err != nil || got != "Hello alice" {
		t.Errorf("Render = %q, %v", got, err)
	}
}

func TestPackageMustRender(t *testing.T) {
	if got := MustRender("Hi {{.Name}}", greetingData{Name: "bob"}); got != "Hi bob" {
		t.Errorf("MustRender = %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("expected panic on bad template")
		}
	}()
	_ = MustRender("{{.bad", struct{}{})
}

func BenchmarkRendererReusableRender(b *testing.B) {
	renderer := NewRenderer(greetingData{Name: "world"}).SetTemplate("Hi {{.Name}}")
	if _, err := renderer.Render(); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_, _ = renderer.Render()
	}
}

func BenchmarkPackageRender(b *testing.B) {
	for b.Loop() {
		_, _ = Render("Hi {{.Name}}", greetingData{Name: "world"})
	}
}
