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

type countingFacadeValidator struct {
	nameCalls int
}

func (v *countingFacadeValidator) Name() string {
	v.nameCalls++
	return "counting-validator"
}

func (*countingFacadeValidator) Validate(Descriptor) error { return nil }

func TestNewEngineReadsExtensionNameOnce(t *testing.T) {
	validator := new(countingFacadeValidator)
	engine, err := NewEngine(EngineConfig{Extensions: []Extension{validator}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine == nil {
		t.Fatal("engine = nil")
	}
	if validator.nameCalls != 1 {
		t.Fatalf("Name calls = %d, want 1", validator.nameCalls)
	}
}

type reservedFacadeValidator struct{}

func (reservedFacadeValidator) Name() string              { return "goap" }
func (reservedFacadeValidator) Validate(Descriptor) error { return nil }

func TestNewEngineRejectsBuiltInPlannerNameCollision(t *testing.T) {
	engine, err := NewEngine(EngineConfig{Extensions: []Extension{reservedFacadeValidator{}}})
	if engine != nil {
		t.Fatalf("engine = %#v, want nil", engine)
	}
	if err == nil || !strings.Contains(err.Error(), `extension "goap" already registered`) {
		t.Fatalf("error = %v, want reserved-name collision", err)
	}
}
