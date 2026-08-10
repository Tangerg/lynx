package client

import (
	"strings"
	"testing"
)

func TestStartRunValidationOwnsMessageAndIdempotencyInvariants(t *testing.T) {
	attachment := Attachment{ID: "att_1", Kind: AttachmentText, Name: "a.txt", Path: "/a.txt"}
	valid := StartRun{RequestID: "req_1", SessionID: "session_1", Message: Message{Text: "inspect", Attachments: []Attachment{attachment}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}

	invalid := valid
	invalid.RequestID = "contains space"
	invalid.SessionID = ""
	invalid.Message.Attachments = append(invalid.Message.Attachments, attachment)
	err := invalid.Validate()
	if err == nil {
		t.Fatal("invalid start request was accepted")
	}
	for _, want := range []string{"request id", "session id", "repeats attachment"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %q", err, want)
		}
	}
}

func TestMessageAttachmentLimitIsCentralized(t *testing.T) {
	attachments := make([]Attachment, MaxMessageAttachments+1)
	for i := range attachments {
		attachments[i] = Attachment{ID: "id", Kind: AttachmentFile, Name: "file", Path: "/file"}
	}
	if err := (Message{Attachments: attachments}).Validate(); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("attachment limit error = %v", err)
	}
}

func TestModelAndApprovalCatalogValidationRejectDuplicates(t *testing.T) {
	models := []Model{
		{ID: "one", DisplayName: "One", Default: true, Efforts: []string{"low"}},
		{ID: "one", DisplayName: "Other", Default: true, Efforts: []string{"medium"}},
	}
	if err := ValidateModels(models); err == nil {
		t.Fatal("duplicate model catalog was accepted")
	}
	rule := ApprovalRule{ID: "rule_1", Rule: "edit:*", Decision: ApprovalAllow, Scope: RememberGlobal}
	if err := ValidateApprovalRules([]ApprovalRule{rule, rule}); err == nil {
		t.Fatal("duplicate approval rules were accepted")
	}
}

func TestCancelRunRequiresExactlyOneStableIdentity(t *testing.T) {
	valid := []CancelRun{
		{RunID: "run_1"},
		{SessionID: "session_1", RequestID: "request_1"},
	}
	for _, request := range valid {
		if err := request.Validate(); err != nil {
			t.Fatalf("valid cancellation %+v: %v", request, err)
		}
	}

	invalid := []CancelRun{
		{},
		{RunID: "run_1", SessionID: "session_1", RequestID: "request_1"},
		{SessionID: "session_1"},
		{RequestID: "request_1"},
		{SessionID: "session_1", RequestID: "contains space"},
	}
	for _, request := range invalid {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid cancellation was accepted: %+v", request)
		}
	}
}
