package agentexec

import (
	"io"
	"reflect"
	"slices"
	"testing"
)

func TestEnginePublicSurfaceIsExecutionOnly(t *testing.T) {
	engineType := reflect.TypeFor[*Engine]()
	var methods []string
	for index := range engineType.NumMethod() {
		methods = append(methods, engineType.Method(index).Name)
	}
	slices.Sort(methods)
	want := []string{"RestoreTurn", "ResumableProcess", "StartTurn"}
	if !slices.Equal(methods, want) {
		t.Fatalf("Engine methods = %v, want execution-only surface %v", methods, want)
	}
	if engineType.Implements(reflect.TypeFor[io.Closer]()) {
		t.Fatal("Engine implements io.Closer; capability resources must be owned by bootstrap.Host")
	}
}

func TestEngineConfigHasNoApplicationOrResourceProxies(t *testing.T) {
	configType := reflect.TypeFor[Config]()
	for _, field := range []string{
		"Steering",
		"Compactor",
		"Extractor",
		"Tools",
		"MCPStatusReader",
		"MCPToolCatalog",
		"MCPConnectionCommands",
		"MCPRegistryCommands",
		"Closers",
		"SkillsGlobalDir",
	} {
		if _, ok := configType.FieldByName(field); ok {
			t.Errorf("Config still exposes %s; it belongs to the direct consumer or Host", field)
		}
	}
}
