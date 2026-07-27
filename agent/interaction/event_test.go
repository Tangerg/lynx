package interaction_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

func TestEventWrapsNestedProtocolValidation(t *testing.T) {
	event := interaction.Event{
		Kind:    interaction.EventModelRequest,
		Round:   1,
		Request: &chat.Request{},
	}
	if err := event.Validate(); !errors.Is(err, interaction.ErrInvalidEvent) {
		t.Fatalf("Validate error = %v, want ErrInvalidEvent", err)
	}
}

func TestEventCloneOwnsNestedProtocolValues(t *testing.T) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("request")))
	if err != nil {
		t.Fatal(err)
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("response"))
	response, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	event := interaction.Event{
		Kind:     interaction.EventModelResponse,
		Round:    1,
		Response: response,
	}
	cloned := event.Clone()
	cloned.Response.Choices[0].Message.Parts[0].Text = "mutated"
	if event.Response.Text() != "response" {
		t.Fatalf("mutating cloned response changed source: %q", event.Response.Text())
	}

	resumeEvent := interaction.Event{
		Kind:  interaction.EventResume,
		Round: 1,
		Resume: &interaction.Resume{
			ID:    "resume-1",
			Input: json.RawMessage(`{"approved":true}`),
		},
	}
	clonedResume := resumeEvent.Clone()
	clonedResume.Resume.Input[2] = 'x'
	if string(resumeEvent.Resume.Input) != `{"approved":true}` {
		t.Fatalf("mutating cloned resume changed source: %s", resumeEvent.Resume.Input)
	}

	suspensionEvent := interaction.Event{
		Kind:  interaction.EventPause,
		Round: 1,
		Suspension: &interaction.Suspension{
			SchemaVersion: interaction.SuspensionSchemaVersion,
			ID:            "pause-1",
			Kind:          interaction.SuspensionHuman,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     time.Unix(1, 0),
		},
	}
	clonedSuspension := suspensionEvent.Clone()
	clonedSuspension.Suspension.Prompt[1] = 'x'
	if string(suspensionEvent.Suspension.Prompt) != `"continue?"` {
		t.Fatalf("mutating cloned suspension changed source: %s", suspensionEvent.Suspension.Prompt)
	}

	requestEvent := interaction.Event{Kind: interaction.EventModelRequest, Round: 1, Request: request}
	clonedRequest := requestEvent.Clone()
	clonedRequest.Request.Messages[0].Parts[0].Text = "mutated"
	if requestEvent.Request.Messages[0].Text() != "request" {
		t.Fatalf("mutating cloned request changed source: %q", requestEvent.Request.Messages[0].Text())
	}
}

func TestResumeEventPreservesValidationCause(t *testing.T) {
	event := interaction.Event{
		Kind:   interaction.EventResume,
		Round:  1,
		Resume: &interaction.Resume{ID: " invalid", Input: json.RawMessage(`true`)},
	}
	if err := event.Validate(); !errors.Is(err, interaction.ErrInvalidID) {
		t.Fatalf("Validate error = %v, want ErrInvalidID", err)
	}
}

func TestValidateIDRejectsUnstableIdentity(t *testing.T) {
	for _, id := range []string{"", "   ", " approval-1", "approval-1 "} {
		if err := interaction.ValidateID(id); !errors.Is(err, interaction.ErrInvalidID) {
			t.Errorf("ValidateID(%q) error = %v", id, err)
		}
	}
	if err := interaction.ValidateID("approval-1"); err != nil {
		t.Fatalf("ValidateID: %v", err)
	}
}

func TestStopReasonValid(t *testing.T) {
	for _, reason := range []interaction.StopReason{interaction.StopNone, interaction.StopBudget, interaction.StopSteps} {
		if !reason.Valid() {
			t.Errorf("StopReason(%q) is invalid", reason)
		}
	}
	if interaction.StopReason("budget+steps").Valid() {
		t.Fatal("unknown stop reason is valid")
	}
}

func TestLimitsValidateRejectsNegativeLocalLimits(t *testing.T) {
	for _, limits := range []interaction.Limits{
		{MaxRounds: -1},
		{MaxConcurrentToolCalls: -1},
		{MaxSteps: -1},
	} {
		if err := limits.Validate(); !errors.Is(err, interaction.ErrInvalidLimits) {
			t.Fatalf("Validate(%+v) error = %v, want ErrInvalidLimits", limits, err)
		}
	}
}
