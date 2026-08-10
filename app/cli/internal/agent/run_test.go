package agent

import (
	"strings"
	"testing"
)

func TestAttachmentValidationRejectsEveryMalformedField(t *testing.T) {
	attachment := Attachment{Kind: "unknown", Size: -1}
	err := attachment.Validate()
	if err == nil {
		t.Fatal("malformed attachment was accepted")
	}
	for _, want := range []string{"id is empty", "kind", "name is empty", "path is empty", "size is negative"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not include %q", err, want)
		}
	}
	valid := Attachment{ID: "att_1", Kind: AttachmentText, Name: "a.txt", Path: "/tmp/a.txt", Size: 0}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid attachment: %v", err)
	}
}

func TestToolCallValidationUsesClosedSemanticKinds(t *testing.T) {
	invalid := ToolCall{Kind: "provider-specific", Status: "maybe"}
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "kind") || !strings.Contains(err.Error(), "status") {
		t.Fatalf("invalid tool error = %v", err)
	}
	if err := (ToolCall{Kind: ToolUnknown, Name: "custom", Status: ToolOK}).Validate(); err != nil {
		t.Fatalf("named custom tool: %v", err)
	}
	if err := (ToolCall{Kind: ToolShell, Status: ToolRunning}).Validate(); err != nil {
		t.Fatalf("running shell tool: %v", err)
	}
	if err := (ToolCall{Kind: ToolShell, Status: ToolCanceled}).Validate(); err != nil {
		t.Fatalf("canceled shell tool: %v", err)
	}
}
