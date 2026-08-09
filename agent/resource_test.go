package agent

import "testing"

func TestResourceQuantitiesFitWithoutUnsignedOverflow(t *testing.T) {
	const maxUint64 = ^uint64(0)
	tests := []struct {
		name       string
		limit      uint64
		quantities []uint64
		want       bool
	}{
		{name: "exact boundary", limit: maxUint64, quantities: []uint64{maxUint64 - 1, 1}, want: true},
		{name: "overflowing sum", limit: maxUint64, quantities: []uint64{maxUint64 - 1, 1, 1}},
		{name: "multiple reservations fit", limit: maxUint64, quantities: []uint64{maxUint64 - 2, 1, 1}, want: true},
		{name: "ordinary over limit", limit: 10, quantities: []uint64{7, 4}},
		{name: "ordinary within limit", limit: 10, quantities: []uint64{7, 3}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resourceQuantitiesFit(test.limit, test.quantities...); got != test.want {
				t.Fatalf("resourceQuantitiesFit(%d, %v) = %t, want %t", test.limit, test.quantities, got, test.want)
			}
		})
	}
}

func TestSaturatingCountAddPreservesMonotonicity(t *testing.T) {
	const maxUint64 = ^uint64(0)
	if got := saturatingCountAdd(maxUint64-1, 2); got != maxUint64 {
		t.Fatalf("saturatingCountAdd() = %d, want %d", got, uint64(maxUint64))
	}
}
