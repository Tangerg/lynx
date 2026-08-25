package openai_test

import (
	"testing"

	"github.com/Tangerg/lynx/models/protocol/openai"
)

func TestReasoningDetailsConfigValidate(t *testing.T) {
	valid := openai.ReasoningDetailsConfig{
		Provider:     "provider",
		TextField:    "reasoning",
		DetailsField: "reasoning_details",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := (openai.ReasoningDetailsConfig{}).Validate(); err == nil {
		t.Fatal("zero ReasoningDetailsConfig.Validate() error = nil")
	}
}
