package text

import (
	"strings"
	"testing"
)

func TestNewRenderer(t *testing.T) {
	r := NewRenderer()
	if r == nil {
		t.Fatal("nil renderer")
	}
	got, err := r.Render()
	if err != nil || got != "" {
		t.Errorf("empty Render = (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestRenderer_WithTemplate(t *testing.T) {
	r := NewRenderer().WithTemplate("Hello {{.name}}").WithVariable("name", "world")
	got, err := r.Render()
	if err != nil || got != "Hello world" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestRenderer_WithVariablesReplaces(t *testing.T) {
	r := NewRenderer().WithTemplate("{{.a}}-{{.b}}")
	r.WithVariable("a", "x")
	r.WithVariables(map[string]any{"b": "y"})
	got, err := r.Render()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// "a" was cleared by WithVariables.
	if got != "<no value>-y" {
		t.Errorf("got %q, want %q", got, "<no value>-y")
	}
}

func TestRenderer_WithDelimiters(t *testing.T) {
	got, err := NewRenderer().
		WithDelimiters("[[", "]]").
		WithTemplate("Hello [[.name]]").
		WithVariable("name", "alice").
		Render()
	if err != nil || got != "Hello alice" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestRenderer_Reset(t *testing.T) {
	r := NewRenderer().WithTemplate("X").WithVariable("k", "v")
	r.Reset()
	got, err := r.Render()
	if err != nil || got != "" {
		t.Errorf("after Reset got %q err %v", got, err)
	}
}

func TestRenderer_Clone(t *testing.T) {
	a := NewRenderer().WithTemplate("Hi {{.name}}").WithVariable("name", "alice")
	if _, err := a.Render(); err != nil {
		t.Fatalf("prime: %v", err)
	}
	b := a.Clone()
	a.WithVariable("name", "bob")
	got, _ := b.Render()
	if got != "Hi alice" {
		t.Errorf("clone reflected mutation: got %q", got)
	}
}

func TestRenderer_Cache(t *testing.T) {
	r := NewRenderer().WithTemplate("Hi {{.n}}").WithVariable("n", "first")
	if _, err := r.Render(); err != nil {
		t.Fatal(err)
	}
	// Cache hit: bypass execute.
	got, _ := r.Render()
	if got != "Hi first" {
		t.Errorf("cached got %q", got)
	}
	// Mutation invalidates cache.
	r.WithVariable("n", "second")
	got, _ = r.Render()
	if got != "Hi second" {
		t.Errorf("after mutation got %q", got)
	}
}

func TestRenderer_Render_ParseError(t *testing.T) {
	_, err := NewRenderer().WithTemplate("{{.bad").Render()
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestRenderer_MustRender_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	NewRenderer().WithTemplate("{{.bad").MustRender()
}

func TestRenderer_RequireVariables(t *testing.T) {
	r := NewRenderer().WithTemplate("Hi {{.name}}, you are {{.age}}")
	if err := r.RequireVariables("name", "age"); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	err := r.RequireVariables("name", "missing")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("err = %v", err)
	}
}

func TestRenderer_RequireVariables_CustomDelimiters(t *testing.T) {
	r := NewRenderer().WithDelimiters("[[", "]]").WithTemplate("Hi [[.name]]")
	if err := r.RequireVariables("name"); err != nil {
		t.Errorf("err = %v", err)
	}
	if err := r.RequireVariables("missing"); err == nil {
		t.Error("expected error")
	}
}

func TestPackageRender(t *testing.T) {
	got, err := Render("Hello {{.name}}", map[string]any{"name": "alice"})
	if err != nil || got != "Hello alice" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestPackageMustRender(t *testing.T) {
	got := MustRender("Hi {{.name}}", map[string]any{"name": "bob"})
	if got != "Hi bob" {
		t.Errorf("got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("expected panic on bad template")
		}
	}()
	_ = MustRender("{{.bad", nil)
}

func BenchmarkRenderer_CachedRender(b *testing.B) {
	r := NewRenderer().WithTemplate("Hi {{.name}}").WithVariable("name", "world")
	if _, err := r.Render(); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_, _ = r.Render()
	}
}

func BenchmarkPackageRender(b *testing.B) {
	for b.Loop() {
		_, _ = Render("Hi {{.name}}", map[string]any{"name": "world"})
	}
}
