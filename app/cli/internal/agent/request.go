package agent

import (
	"errors"
	"fmt"
	"strings"
)

const MaxMessageAttachments = 16

func (s StartRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(s.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := s.Message.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := s.Options.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("start run: %w", err)
	}
	return nil
}

func (d DeleteSession) Validate() error {
	var problems []error
	if d.CommandID != "" {
		if err := d.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(d.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

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
		if strings.TrimSpace(attachment.Path) == "" {
			return fmt.Errorf("message attachment %d: local path is empty", i+1)
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

func (s SubscribeRun) Validate() error {
	if strings.TrimSpace(s.RunID) == "" {
		return errors.New("subscribe run: run id is empty")
	}
	if strings.TrimSpace(s.SegmentID) == "" {
		return errors.New("subscribe run: segment id is empty")
	}
	return nil
}

func (r ResumeRun) Validate() error {
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
	}
	if strings.TrimSpace(r.RunID) == "" {
		return errors.New("resume run: run id is empty")
	}
	if len(r.Answers) == 0 {
		return errors.New("resume run: answers are empty")
	}
	seen := make(map[string]struct{}, len(r.Answers))
	for i, response := range r.Answers {
		id := strings.TrimSpace(response.ItemID)
		if id == "" {
			return fmt.Errorf("resume run: answer %d has no item id", i+1)
		}
		if response.Answer == nil {
			return fmt.Errorf("resume run: answer %d is nil", i+1)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("resume run: item %q is answered more than once", id)
		}
		seen[id] = struct{}{}
	}
	if r.Message != nil {
		if err := r.Message.Validate(); err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
	}
	return nil
}

func (c CancelRun) Validate() error {
	if c.CommandID != "" {
		if err := c.CommandID.Validate(); err != nil {
			return fmt.Errorf("cancel run: %w", err)
		}
	}
	if strings.TrimSpace(c.RunID) == "" {
		return errors.New("cancel run: run id is empty")
	}
	return nil
}

func (s SteerRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(s.RunID) == "" {
		problems = append(problems, errors.New("run id is empty"))
	}
	if strings.TrimSpace(s.SegmentID) == "" {
		problems = append(problems, errors.New("segment id is empty"))
	}
	if err := s.Message.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("steer run: %w", err)
	}
	return nil
}

func (s SegmentStream) Validate() error {
	var problems []error
	if strings.TrimSpace(s.RunID) == "" {
		problems = append(problems, errors.New("run id is empty"))
	}
	if strings.TrimSpace(s.SegmentID) == "" {
		problems = append(problems, errors.New("segment id is empty"))
	}
	if s.Events == nil {
		problems = append(problems, errors.New("event stream is nil"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("segment stream: %w", err)
	}
	return nil
}

// ValidateStart enforces the runs.start-specific response invariant: every
// accepted start creates and names its opening user item.
func (s SegmentStream) ValidateStart() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.UserItemID) == "" {
		return errors.New("start segment stream: user item id is empty")
	}
	return nil
}

// ValidateResume enforces both the target identity and the runs.resume response
// union. UserItemID exists exactly when an optional continuation message was
// committed with the answers.
func (s SegmentStream) ValidateResume(runID string, message *Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.RunID != strings.TrimSpace(runID) {
		return fmt.Errorf("resume segment stream: run %q does not match %q", s.RunID, runID)
	}
	hasUserItem := strings.TrimSpace(s.UserItemID) != ""
	if hasUserItem != (message != nil) {
		return errors.New("resume segment stream: user item id does not match input presence")
	}
	return nil
}

// ValidateSubscription enforces that rebinding an existing segment creates no
// user item of its own.
func (s SegmentStream) ValidateSubscription() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.UserItemID != "" {
		return errors.New("subscription segment stream carries a user item id")
	}
	return nil
}
