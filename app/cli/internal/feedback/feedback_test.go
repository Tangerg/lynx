package feedback

import "testing"

func TestSignalRequiresContent(t *testing.T) {
	if err := (Signal{Rating: Positive}).Validate(); err != nil {
		t.Fatalf("rated signal: %v", err)
	}
	if err := (Signal{Text: "details"}).Validate(); err != nil {
		t.Fatalf("text signal: %v", err)
	}
	if err := (Signal{}).Validate(); err == nil {
		t.Fatal("accepted empty signal")
	}
}
