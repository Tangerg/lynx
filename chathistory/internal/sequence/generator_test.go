package sequence

import "testing"

func TestGeneratorReservesMonotonicRangesAcrossClockRegression(t *testing.T) {
	var generator Generator
	if first := generator.reserveAt(100, 3); first != 100 {
		t.Fatalf("first range starts at %d, want 100", first)
	}
	if second := generator.reserveAt(90, 2); second != 103 {
		t.Fatalf("second range starts at %d, want 103", second)
	}
	if third := generator.reserveAt(200, 1); third != 200 {
		t.Fatalf("third range starts at %d, want 200", third)
	}
}
