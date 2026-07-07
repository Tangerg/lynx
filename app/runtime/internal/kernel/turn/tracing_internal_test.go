package turn

import "testing"

func TestTurnTracerNameUsesTurnBoundary(t *testing.T) {
	if turnTracerName != "lynx/lyra/turn" {
		t.Fatalf("turn tracer name = %q, want lynx/lyra/turn", turnTracerName)
	}
}
