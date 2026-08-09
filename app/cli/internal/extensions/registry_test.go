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
	loaded, err := Load(registry, Plugin{ID: "formats", Setup: func(scope *Scope) error {
		if _, err := Contribute(scope, point, format{ID: "json", Label: "JSON"}, Contribution{Order: 20}); err != nil {
			return err
		}
		_, err := Contribute(scope, point, format{ID: "markdown", Label: "Markdown"}, Contribution{Order: 10})
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	values := Values(registry, point)
	if len(values) != 2 || values[0].ID != "markdown" || values[1].ID != "json" {
		t.Fatalf("values = %+v", values)
	}
	loaded.Dispose()
	if values := Values(registry, point); len(values) != 0 {
		t.Fatalf("values after unload = %+v", values)
	}
}

func TestSetupFailureRollsBackEarlierContributions(t *testing.T) {
	point := NewMultiPoint[func()]("test.hook")
	registry := new(Registry)
	want := errors.New("setup failed")
	_, err := Load(registry, Plugin{ID: "broken", Setup: func(scope *Scope) error {
		if _, err := Contribute(scope, point, func() {}, Contribution{}); err != nil {
			return err
		}
		return want
	}})
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
	first, err := Load(registry, Plugin{ID: "first", Setup: func(scope *Scope) error {
		_, err := Contribute(scope, point, format{ID: "json"}, Contribution{})
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Dispose()

	if _, err := Load(registry, Plugin{ID: "second", Setup: func(scope *Scope) error {
		_, err := Contribute(scope, point, format{ID: "json"}, Contribution{})
		return err
	}}); err == nil {
		t.Fatal("duplicate key was accepted")
	}
}

func TestPointsWithTheSameIDCannotDisagreeOnType(t *testing.T) {
	stringsPoint := NewMultiPoint[string]("test.same")
	intsPoint := NewMultiPoint[int]("test.same")
	registry := new(Registry)
	loaded, err := Load(registry, Plugin{ID: "strings", Setup: func(scope *Scope) error {
		_, err := Contribute(scope, stringsPoint, "one", Contribution{})
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Dispose()

	if _, err := Load(registry, Plugin{ID: "ints", Setup: func(scope *Scope) error {
		_, err := Contribute(scope, intsPoint, 1, Contribution{})
		return err
	}}); err == nil {
		t.Fatal("incompatible point definition was accepted")
	}
}

func TestPluginMustUnloadBeforeItCanReload(t *testing.T) {
	registry := new(Registry)
	plugin := Plugin{ID: "same", Setup: func(*Scope) error { return nil }}
	loaded, err := Load(registry, plugin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(registry, plugin); err == nil {
		t.Fatal("loaded the same plugin twice")
	}
	loaded.Dispose()
	loaded, err = Load(registry, plugin)
	if err != nil {
		t.Fatalf("reload after unload: %v", err)
	}
	loaded.Dispose()
}
