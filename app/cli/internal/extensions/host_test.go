package extensions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func manifest(id string, setup func(*Scope) error) Plugin {
	return Plugin{ID: id, Version: "1.0.0", APIVersion: HostAPIVersion, Setup: setup}
}

func TestCapabilityProtectedPointDefaultsRestrictedPluginsToDeny(t *testing.T) {
	point := NewCapabilityKeyedPoint("test.command", Capability("commands"), func(value format) string { return value.ID })
	registry := new(Registry)
	denied := manifest("test.denied", func(scope *Scope) error {
		_, err := scope.Contribute(point, format{ID: "hello"}, Contribution{})
		return err
	})
	if _, err := Load(registry, denied); err == nil {
		t.Fatal("restricted plugin contributed without declaring the capability")
	}

	allowed := manifest("test.allowed", func(scope *Scope) error {
		_, err := scope.Contribute(point, format{ID: "hello"}, Contribution{})
		return err
	})
	allowed.Capabilities = []Capability{"commands"}
	loaded, err := Load(registry, allowed)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Dispose()
	if values := registry.Values(point); len(values) != 1 || values[0].ID != "hello" {
		t.Fatalf("values = %+v", values)
	}
}

func TestManifestValidationRejectsIncompatibleOrAmbiguousMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Plugin)
	}{
		{"id", func(plugin *Plugin) { plugin.ID = "Bad ID" }},
		{"version", func(plugin *Plugin) { plugin.Version = "latest" }},
		{"api", func(plugin *Plugin) { plugin.APIVersion++ }},
		{"self dependency", func(plugin *Plugin) { plugin.Requires = []string{plugin.ID} }},
		{"duplicate dependency", func(plugin *Plugin) { plugin.Requires = []string{"test.base", "test.base"} }},
		{"duplicate capability", func(plugin *Plugin) { plugin.Capabilities = []Capability{"commands", "commands"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := manifest("test.valid", func(*Scope) error { return nil })
			tt.mutate(&plugin)
			if err := ValidateManifest(plugin); err == nil {
				t.Fatalf("manifest was accepted: %+v", plugin)
			}
		})
	}
	invalid := Plugin{ID: "test.partial", Setup: func(*Scope) error { return nil }}
	if _, err := Load(new(Registry), invalid); err == nil {
		t.Fatal("Load bypassed full manifest validation")
	}
}

func TestHostRejectsDuplicatePluginIdentity(t *testing.T) {
	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	plugin := manifest("test.duplicate", func(*Scope) error { return nil })
	results, err := host.Activate([]Plugin{plugin, plugin})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Phase != PluginSkipped || results[0].Err == nil {
		t.Fatalf("results = %+v", results)
	}
	statuses := host.Statuses()
	if len(statuses) != 1 || statuses[0].ID != plugin.ID || statuses[0].Phase != PluginSkipped {
		t.Fatalf("statuses = %+v", statuses)
	}
}

type failingSource struct{ err error }

func (failingSource) ID() string { return "broken" }
func (f failingSource) Discover(context.Context) (SourceResult, error) {
	return SourceResult{}, f.err
}

func TestDiscoveryPreservesSourceOrderAndIsolatesFailures(t *testing.T) {
	want := errors.New("offline")
	result, err := Discover(t.Context(),
		StaticSource{Name: "first", Plugins: []Plugin{manifest("test.first", func(*Scope) error { return nil })}},
		failingSource{err: want},
		StaticSource{Name: "last", Plugins: []Plugin{manifest("test.last", func(*Scope) error { return nil })}},
	)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{result.Plugins[0].ID, result.Plugins[1].ID}
	if !slices.Equal(ids, []string{"test.first", "test.last"}) {
		t.Fatalf("plugin order = %v", ids)
	}
	if len(result.Issues) != 1 || result.Issues[0].Source != "broken" || !errors.Is(result.Issues[0].Err, want) {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestHostOrdersDependenciesAndReloadsTheirClosure(t *testing.T) {
	registry := new(Registry)
	host, err := NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	point := NewMultiPoint[string]("test.lifecycle")
	var lifecycle []string
	results, err := host.Activate([]Plugin{
		lifecyclePlugin(point, &lifecycle, "test.dependent", "test.base"),
		lifecyclePlugin(point, &lifecycle, "test.independent"),
		lifecyclePlugin(point, &lifecycle, "test.base"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allLoaded(results) {
		t.Fatalf("activation = %+v", results)
	}
	requireLifecycle(t, lifecycle, []string{"load:test.independent", "load:test.base", "load:test.dependent"})
	requireAffected(t, host, "test.base", []string{"test.base", "test.dependent"})

	lifecycle = nil
	results, err = host.Reload("test.base")
	if err != nil || !allLoaded(results) {
		t.Fatalf("reload = %+v, %v", results, err)
	}
	requireLifecycle(t, lifecycle, []string{
		"unload:test.dependent", "unload:test.base",
		"load:test.base", "load:test.dependent",
	})
	if values := registry.Values(point); len(values) != 3 {
		t.Fatalf("reload left %d contributions, want 3", len(values))
	}
}

func requireLifecycle(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("lifecycle = %v, want %v", got, want)
	}
}

func requireAffected(t *testing.T, host *Host, pluginID string, want []string) {
	t.Helper()
	got, err := host.Affected(pluginID)
	if err != nil {
		t.Fatal(err)
	}
	requireLifecycle(t, got, want)
}

func lifecyclePlugin(point Point[string], lifecycle *[]string, id string, requires ...string) Plugin {
	item := manifest(id, func(scope *Scope) error {
		*lifecycle = append(*lifecycle, "load:"+id)
		if err := scope.OnDispose(func() error {
			*lifecycle = append(*lifecycle, "unload:"+id)
			return nil
		}); err != nil {
			return err
		}
		_, err := scope.Contribute(point, id, Contribution{})
		return err
	})
	item.Requires = requires
	return item
}

func TestHostSkipsMissingCyclesAndDependentsOfFailedSetup(t *testing.T) {
	broken := manifest("test.broken", func(*Scope) error { return errors.New("boom") })
	dependent := manifest("test.dependent", func(*Scope) error { return nil })
	dependent.Requires = []string{"test.broken"}
	missing := manifest("test.missing", func(*Scope) error { return nil })
	missing.Requires = []string{"test.absent"}
	cycleA := manifest("test.cycle-a", func(*Scope) error { return nil })
	cycleB := manifest("test.cycle-b", func(*Scope) error { return nil })
	cycleA.Requires = []string{cycleB.ID}
	cycleB.Requires = []string{cycleA.ID}

	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	results, err := host.Activate([]Plugin{dependent, broken, missing, cycleA, cycleB})
	if err != nil {
		t.Fatal(err)
	}
	phases := make(map[string]LifecyclePhase)
	for _, result := range results {
		phases[result.PluginID] = result.Phase
	}
	if phases[broken.ID] != PluginFailed || phases[dependent.ID] != PluginSkipped || phases[missing.ID] != PluginSkipped || phases[cycleA.ID] != PluginSkipped || phases[cycleB.ID] != PluginSkipped {
		t.Fatalf("phases = %+v", phases)
	}
}

func TestSetupRollbackRunsOwnedCleanup(t *testing.T) {
	registry := new(Registry)
	cleaned := false
	plugin := manifest("test.rollback", func(scope *Scope) error {
		if err := scope.OnDispose(func() error { cleaned = true; return nil }); err != nil {
			return err
		}
		return errors.New("setup failed")
	})
	if _, err := Load(registry, plugin); err == nil {
		t.Fatal("setup failure was accepted")
	}
	if !cleaned {
		t.Fatal("owned cleanup did not run during rollback")
	}
}

func TestCleanupFailuresAreJoinedAfterEveryCleanupRuns(t *testing.T) {
	firstFailure := errors.New("first cleanup failed")
	lastFailure := errors.New("last cleanup failed")
	var order []string
	plugin := manifest("test.cleanup-errors", func(scope *Scope) error {
		for _, cleanup := range []struct {
			name string
			err  error
		}{
			{name: "first", err: firstFailure},
			{name: "middle"},
			{name: "last", err: lastFailure},
		} {
			if err := scope.OnDispose(func() error {
				order = append(order, cleanup.name)
				return cleanup.err
			}); err != nil {
				return err
			}
		}
		return nil
	})
	loaded, err := Load(new(Registry), plugin)
	if err != nil {
		t.Fatal(err)
	}
	disposeErr := loaded.Dispose()
	if !errors.Is(disposeErr, firstFailure) || !errors.Is(disposeErr, lastFailure) {
		t.Fatalf("Dispose error = %v", disposeErr)
	}
	if !slices.Equal(order, []string{"last", "middle", "first"}) {
		t.Fatalf("cleanup order = %v", order)
	}
	if repeated := loaded.Dispose(); !errors.Is(repeated, firstFailure) || !errors.Is(repeated, lastFailure) || repeated.Error() != disposeErr.Error() {
		t.Fatalf("repeated Dispose returned %v, want stable %v", repeated, disposeErr)
	}
}

func TestHostUnloadSurfacesCleanupFailureAfterReleasingContributions(t *testing.T) {
	want := errors.New("cleanup failed")
	point := NewMultiPoint[string]("test.cleanup-point")
	registry := new(Registry)
	host, err := NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	plugin := manifest("test.cleanup-host", func(scope *Scope) error {
		if _, err := scope.Contribute(point, "owned", Contribution{}); err != nil {
			return err
		}
		return scope.OnDispose(func() error { return want })
	})
	if results, err := host.Activate([]Plugin{plugin}); err != nil || !allLoaded(results) {
		t.Fatalf("Activate = %+v, %v", results, err)
	}
	if err := host.Unload(plugin.ID); !errors.Is(err, want) {
		t.Fatalf("Unload error = %v", err)
	}
	if values := registry.Values(point); len(values) != 0 {
		t.Fatalf("failed cleanup leaked contributions: %v", values)
	}
	requireFailedPluginInfo(t, host, plugin.ID)
	if err := host.Close(); err != nil {
		t.Fatalf("Close repeated an already-released cleanup: %v", err)
	}
}

func requireFailedPluginInfo(t *testing.T, host *Host, pluginID string) {
	t.Helper()
	statuses := host.Statuses()
	if len(statuses) != 1 || statuses[0].ID != pluginID || statuses[0].Phase != PluginFailed || statuses[0].Detail == "" {
		t.Fatalf("plugin info = %+v", statuses)
	}
}

func TestSetupFailureReportsRollbackFailureWithoutLeakingOwnership(t *testing.T) {
	setupFailure := errors.New("setup failed")
	rollbackFailure := errors.New("rollback failed")
	registry := new(Registry)
	plugin := manifest("test.rollback-errors", func(scope *Scope) error {
		if err := scope.OnDispose(func() error { return rollbackFailure }); err != nil {
			return err
		}
		return setupFailure
	})
	_, err := Load(registry, plugin)
	if !errors.Is(err, setupFailure) || !errors.Is(err, rollbackFailure) {
		t.Fatalf("Load error = %v", err)
	}
	loaded, err := Load(registry, manifest(plugin.ID, func(*Scope) error { return nil }))
	if err != nil {
		t.Fatalf("rollback leaked the plugin claim: %v", err)
	}
	if err := loaded.Dispose(); err != nil {
		t.Fatal(err)
	}
}

func TestPluginPanicsAreIsolatedDuringSetupAndCleanup(t *testing.T) {
	registry := new(Registry)
	setupPanic := manifest("test.setup-panic", func(*Scope) error { panic("setup boom") })
	if _, err := Load(registry, setupPanic); err == nil {
		t.Fatal("setup panic escaped as success")
	}

	var cleaned bool
	cleanupPanic := manifest("test.cleanup-panic", func(scope *Scope) error {
		if err := scope.OnDispose(func() error { cleaned = true; return nil }); err != nil {
			return err
		}
		return scope.OnDispose(func() error { panic("cleanup boom") })
	})
	loaded, err := Load(registry, cleanupPanic)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Dispose(); err == nil {
		t.Fatal("cleanup panic was not surfaced")
	}
	if !cleaned {
		t.Fatal("one cleanup panic prevented the remaining cleanup")
	}
}

func TestHostDoesNotHoldStateLockWhileCallingPluginCode(t *testing.T) {
	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	plugin := manifest("test.introspect", func(scope *Scope) error {
		_ = host.Statuses()
		return scope.OnDispose(func() error { _ = host.Statuses(); return nil })
	})
	done := make(chan error, 1)
	go func() {
		_, err := host.Activate([]Plugin{plugin})
		if err == nil {
			host.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("plugin setup or cleanup deadlocked on host state")
	}
}

func allLoaded(results []LifecycleResult) bool {
	return len(results) > 0 && !slices.ContainsFunc(results, func(result LifecycleResult) bool { return result.Phase != PluginLoaded })
}
