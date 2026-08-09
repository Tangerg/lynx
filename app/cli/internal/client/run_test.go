package client

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
