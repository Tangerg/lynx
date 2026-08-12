package agent

import (
	"errors"
	"testing"
)

func TestAcceptedMutationErrorPreservesReceiptAndCause(t *testing.T) {
	cause := errors.New("invalid receipt")
	receipt := SegmentStream{RunID: "run_1", SegmentID: "seg_1"}
	err := NewAcceptedMutationError(receipt, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("accepted mutation error = %v", err)
	}
	got, ok := AcceptedMutationReceipt(err)
	if !ok || got.RunID != receipt.RunID || got.SegmentID != receipt.SegmentID {
		t.Fatalf("accepted mutation receipt = %+v, %t", got, ok)
	}
	if err := NewAcceptedMutationError(receipt, nil); err != nil {
		t.Fatalf("nil cause produced %v", err)
	}
}
