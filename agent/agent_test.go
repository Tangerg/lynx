package agent

import (
	"strings"
	"testing"
)

type nilFacadeExtension struct{ name string }

func (e *nilFacadeExtension) Name() string { return e.name }

func TestNewEngineReturnsTypedNilExtensionError(t *testing.T) {
	var extension *nilFacadeExtension
	engine, err := NewEngine(EngineConfig{Extensions: []Extension{extension}})
	if engine != nil {
		t.Fatalf("engine = %#v, want nil", engine)
	}
	if err == nil || !strings.Contains(err.Error(), "nil extension") {
		t.Fatalf("error = %v, want nil extension", err)
	}
}
