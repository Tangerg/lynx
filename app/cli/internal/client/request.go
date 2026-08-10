package client

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MaxMessageAttachments is the product-level limit shared by every input
// adapter and runtime implementation.
const MaxMessageAttachments = 16

// Validate checks a complete idempotent start request.
func (r StartRun) Validate() error {
	var problems []error
	if err := validateRequestID(r.RequestID); err != nil {
		problems = append(problems, err)
	}
	if strings.TrimSpace(r.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := r.Message.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := r.Options.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("start run: %w", err)
	}
	return nil
}

// Validate checks a user message and the attachment identities it owns.
func (m Message) Validate() error {
	if strings.TrimSpace(m.Text) == "" && len(m.Attachments) == 0 {
		return errors.New("message is empty")
	}
	if len(m.Attachments) > MaxMessageAttachments {
		return fmt.Errorf("message has %d attachments; limit is %d", len(m.Attachments), MaxMessageAttachments)
	}
	ids := make(map[string]struct{}, len(m.Attachments))
	paths := make(map[string]struct{}, len(m.Attachments))
	for i, attachment := range m.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("message attachment %d: %w", i+1, err)
		}
		if _, duplicate := ids[attachment.ID]; duplicate {
			return fmt.Errorf("message repeats attachment id %q", attachment.ID)
		}
		if _, duplicate := paths[attachment.Path]; duplicate {
			return fmt.Errorf("message repeats attachment path %q", attachment.Path)
		}
		ids[attachment.ID] = struct{}{}
		paths[attachment.Path] = struct{}{}
	}
	return nil
}

// Validate checks a follow subscription request.
func (r FollowRun) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return errors.New("follow run: run id is empty")
	}
	return nil
}

// Validate checks the identity and payload of a resume request. Matching the
// answer to the live interaction remains the runtime's responsibility.
func (r ResumeRun) Validate() error {
	if strings.TrimSpace(r.RunID) == "" {
		return errors.New("resume run: run id is empty")
	}
	if strings.TrimSpace(r.InterruptID) == "" {
		return errors.New("resume run: interrupt id is empty")
	}
	if r.Answer == nil {
		return errors.New("resume run: answer is nil")
	}
	return nil
}

// Validate checks that cancellation names exactly one logical run request.
func (r CancelRun) Validate() error {
	byRun := strings.TrimSpace(r.RunID) != ""
	byRequest := strings.TrimSpace(r.SessionID) != "" || strings.TrimSpace(r.RequestID) != ""
	if byRun == byRequest {
		return errors.New("cancel run: identify either a run or a start request")
	}
	if byRequest {
		if strings.TrimSpace(r.SessionID) == "" {
			return errors.New("cancel run: session id is empty")
		}
		if strings.TrimSpace(r.RequestID) == "" {
			return errors.New("cancel run: request id is empty")
		}
		if err := validateRequestID(r.RequestID); err != nil {
			return fmt.Errorf("cancel run: %w", err)
		}
	}
	return nil
}

func validateRequestID(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > 128 || strings.IndexFunc(id, unicode.IsSpace) >= 0 {
		return fmt.Errorf("request id %q is invalid", id)
	}
	return nil
}
