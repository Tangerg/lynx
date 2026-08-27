package server

import (
	"context"
	"errors"
	"testing"

	feedbackapp "github.com/Tangerg/scope/app/runtime/internal/application/feedback"
	feedbackdomain "github.com/Tangerg/scope/app/runtime/internal/domain/feedback"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

type feedbackRecorderFake struct {
	command feedbackapp.Command
	err     error
}

func (f *feedbackRecorderFake) Record(_ context.Context, command feedbackapp.Command) error {
	f.command = command
	return f.err
}

func TestCreateFeedbackMapsProtocolRequestToRecorder(t *testing.T) {
	recorder := &feedbackRecorderFake{}
	s := &Server{feedback: recorder}
	err := s.CreateFeedback(t.Context(), protocol.FeedbackRequest{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Rating: protocol.FeedbackNegative, Text: "the answer missed the request",
	})
	if err != nil {
		t.Fatalf("CreateFeedback: %v", err)
	}
	if recorder.command != (feedbackapp.Command{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Rating: feedbackdomain.RatingNegative, Text: "the answer missed the request",
	}) {
		t.Fatalf("command = %+v", recorder.command)
	}
}

func TestCreateFeedbackMapsInvalidEntryToInvalidParams(t *testing.T) {
	s := &Server{feedback: &feedbackRecorderFake{err: feedbackdomain.ErrInvalid}}
	err := s.CreateFeedback(t.Context(), protocol.FeedbackRequest{})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("CreateFeedback = %v, want invalid params", err)
	}
}
