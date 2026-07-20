package agent

import (
	"errors"
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

type panickingFacadeExtension struct{ cause error }

func (e panickingFacadeExtension) Name() string { panic(e.cause) }

func TestNewEngineReturnsExtensionNamePanic(t *testing.T) {
	cause := errors.New("name unavailable")
	engine, err := NewEngine(EngineConfig{Extensions: []Extension{panickingFacadeExtension{cause: cause}}})
	if engine != nil {
		t.Fatalf("engine = %#v, want nil", engine)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped cause", err)
	}
}
