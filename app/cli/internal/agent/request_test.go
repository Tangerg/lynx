package agent

import (
	"strings"
	"testing"
)

func TestStartRunValidation(t *testing.T) {
	valid := StartRun{SessionID: "ses_1", Message: Message{Text: "hello"}, Options: RunOptions{Provider: "mock", Model: "balanced"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Options.Model = ""
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "selected together") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteSessionValidatesItsOptionalMutationIdentity(t *testing.T) {
	if err := (DeleteSession{SessionID: "ses_1"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (DeleteSession{CommandID: CommandID("invalid"), SessionID: "ses_1"}).Validate(); err == nil {
		t.Fatal("invalid deletion command identity was accepted")
	}
	if err := (DeleteSession{}).Validate(); err == nil {
		t.Fatal("empty deletion target was accepted")
	}
}

func TestStartRunEqualUsesTheCompleteMutationFingerprint(t *testing.T) {
	request := StartRun{
		CommandID: CommandID("cli_11111111111111111111111111111111"), SessionID: "ses_1",
		Message: Message{Text: "hello"}, Options: RunOptions{Provider: "mock", Model: "balanced"},
	}
	if !request.Equal(request.Clone()) {
		t.Fatal("cloned start request is not equal")
	}
	changed := request.Clone()
	changed.Message.Text = "different"
	if request.Equal(changed) {
		t.Fatal("different start payloads are equal")
	}
}

func TestResumeRunRequiresCompleteUniqueSet(t *testing.T) {
	request := ResumeRun{RunID: "run_1", Answers: []InterruptAnswer{
		{ItemID: "a", Answer: ApprovalAnswer{Decision: ApprovalApprove}},
		{ItemID: "q", Answer: QuestionAnswer{Values: [][]string{{"yes"}}}},
	}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Answers[1].ItemID = "a"
	if err := request.Validate(); err == nil {
		t.Fatal("duplicate interrupt was accepted")
	}
}

func TestSubscribeRunNeedsRunAndSegment(t *testing.T) {
	if err := (SubscribeRun{RunID: "run_1", SegmentID: "seg_1", AfterEventID: "opaque"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SubscribeRun{RunID: "run_1"}).Validate(); err == nil {
		t.Fatal("missing segment was accepted")
	}
}

func TestMessageRejectsDuplicateAttachments(t *testing.T) {
	attachment := Attachment{ID: "a", Kind: AttachmentText, Name: "a.txt", Path: "/tmp/a.txt"}
	message := Message{Attachments: []Attachment{attachment, attachment}}
	if err := message.Validate(); err == nil {
		t.Fatal("duplicate attachment was accepted")
	}
}

func TestDurableAttachmentMayLackLocalPathButDraftMayNot(t *testing.T) {
	durable := Attachment{ID: "item_1:image:0", Kind: AttachmentImage, Name: "image.png", MimeType: "image/png", Size: 8}
	if err := durable.Validate(); err != nil {
		t.Fatalf("durable attachment: %v", err)
	}
	if err := (Message{Attachments: []Attachment{durable}}).Validate(); err == nil || !strings.Contains(err.Error(), "local path") {
		t.Fatalf("draft attachment error = %v", err)
	}
}

func TestSegmentStreamValidatesOperationSpecificUserItemIdentity(t *testing.T) {
	stream := SegmentStream{RunID: "run_1", SegmentID: "seg_1", Events: func(func(RunEvent, error) bool) {}}
	if err := stream.ValidateStart(); err == nil {
		t.Fatal("start stream without a user item id was accepted")
	}
	stream.UserItemID = "item_1"
	if err := stream.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateSubscription(); err == nil {
		t.Fatal("subscription stream with a user item id was accepted")
	}
	if err := stream.ValidateResume("run_1", &Message{Text: "continue"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_1", nil); err == nil {
		t.Fatal("response-only resume with a user item id was accepted")
	}
	stream.UserItemID = ""
	if err := stream.ValidateSubscription(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_1", nil); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_other", nil); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched resume target error = %v", err)
	}
}
