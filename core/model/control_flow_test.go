package model_test

import (
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

type controlFlowErr bool

func (e controlFlowErr) Error() string     { return "control flow" }
func (e controlFlowErr) ControlFlow() bool { return bool(e) }

func TestIsControlFlowError(t *testing.T) {
	if !model.IsControlFlowError(controlFlowErr(true)) {
		t.Fatal("true marker must be control flow")
	}
	if model.IsControlFlowError(controlFlowErr(false)) {
		t.Fatal("false marker must not be control flow")
	}
	if !model.IsControlFlowError(fmt.Errorf("wrapped: %w", controlFlowErr(true))) {
		t.Fatal("wrapped marker must be detected")
	}
	if model.IsControlFlowError(fmt.Errorf("ordinary")) {
		t.Fatal("ordinary errors must not be control flow")
	}
}
