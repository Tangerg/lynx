package hitl

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
)

func TestIsInterruptUsesConcreteAgentSignal(t *testing.T) {
	interrupt := &InterruptError{Key: "approval"}
	if !IsInterrupt(fmt.Errorf("wrapped: %w", interrupt)) {
		t.Fatal("wrapped InterruptError was not recognized")
	}
	if IsInterrupt(&toolloop.AbortError{Err: errors.New("fatal")}) {
		t.Fatal("ordinary tool-loop abort must not be treated as an agent interrupt")
	}
	var abort *toolloop.AbortError
	if !errors.As(interrupt, &abort) {
		t.Fatal("InterruptError did not expose target tool-loop abort control")
	}
}
