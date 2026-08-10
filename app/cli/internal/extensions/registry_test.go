package extensions

import (
	"errors"
	"testing"
)

type format struct {
	ID    string
	Label string
}

func TestKeyedContributionsAreTypedOrderedAndDisposable(t *testing.T) {
	point := NewKeyedPoint("test.format", func(value format) string { return value.ID })
	registry := new(Registry)
	loaded, err := Load(registry, manifest("formats", contributeFormats(point)))
	if err != nil {
		t.Fatal(err)
	}
	values := Values(registry, point)
	if len(values) != 2 || values[0].ID != "markdown" || values[1].ID != "json" {
		t.Fatalf("values = %+v", values)
	}
	owned := OwnedValues(registry, point)
	if len(owned) != 2 || owned[0].PluginID != "formats" || owned[1].PluginID != "formats" {
		t.Fatalf("owned values = %+v", owned)
	}
	if err := loaded.Dispose(); err != nil {
		t.Fatal(err)
	}
	if values := Values(registry, point); len(values) != 0 {
		t.Fatalf("values after unload = %+v", values)
	}
}

func contributeFormats(point Point[format]) func(*Scope) error {
	return func(scope *Scope) error {
		if _, err := Contribute(scope, point, format{ID: "json", Label: "JSON"}, Contribution{Order: 20}); err != nil {
			return err
		}
		_, err := Contribute(scope, point, format{ID: "markdown", Label: "Markdown"}, Contribution{Order: 10})
		return err
	}
}

func TestSetupFailureRollsBackEarlierContributions(t *testing.T) {
	point := NewMultiPoint[func()]("test.hook")
	registry := new(Registry)
	want := errors.New("setup failed")
	_, err := Load(registry, manifest("broken", func(scope *Scope) error {
		if _, err := Contribute(scope, point, func() {}, Contribution{}); err != nil {
			return err
		}
		return want
	}))
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if values := Values(registry, point); len(values) != 0 {
		t.Fatalf("rollback left %d contribution(s)", len(values))
	}
}

func TestKeyedPointRejectsDuplicateOwnership(t *testing.T) {
	point := NewKeyedPoint("test.format", func(value format) string { return value.ID })
	registry := new(Registry)
	first, err := Load(registry, manifest("first", func(scope *Scope) error {
		_, err := Contribute(scope, point, format{ID: "json"}, Contribution{})
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Dispose()

	if _, err := Load(registry, manifest("second", func(scope *Scope) error {
		_, err := Contribute(scope, point, format{ID: "json"}, Contribution{})
		return err
	})); err == nil {
		t.Fatal("duplicate key was accepted")
	}
}

func TestPointsWithTheSameIDCannotDisagreeOnType(t *testing.T) {
	stringsPoint := NewMultiPoint[string]("test.same")
	intsPoint := NewMultiPoint[int]("test.same")
	registry := new(Registry)
	loaded, err := Load(registry, manifest("strings", func(scope *Scope) error {
		_, err := Contribute(scope, stringsPoint, "one", Contribution{})
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Dispose()

	if _, err := Load(registry, manifest("ints", func(scope *Scope) error {
		_, err := Contribute(scope, intsPoint, 1, Contribution{})
		return err
	})); err == nil {
		t.Fatal("incompatible point definition was accepted")
	}
}

func TestPluginMustUnloadBeforeItCanReload(t *testing.T) {
	registry := new(Registry)
	plugin := manifest("same", func(*Scope) error { return nil })
	loaded, err := Load(registry, plugin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(registry, plugin); err == nil {
		t.Fatal("loaded the same plugin twice")
	}
	if err := loaded.Dispose(); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(registry, plugin)
	if err != nil {
		t.Fatalf("reload after unload: %v", err)
	}
	if err := loaded.Dispose(); err != nil {
		t.Fatal(err)
	}
}
