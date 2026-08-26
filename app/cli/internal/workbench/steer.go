package workbench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// PendingSteer owns an instruction and its borrowed composer attachments until
// the runtime definitively accepts or rejects the exact command identity.
// Replay metadata binds cold recovery to the runtime idempotency store that
// first received the command.
type PendingSteer struct {
	SessionID       string         `json:"sessionId"`
	Command         agent.SteerRun `json:"command"`
	StagedAt        time.Time      `json:"stagedAt"`
	ReplayNamespace string         `json:"replayNamespace,omitempty"`
	ReplayUntil     time.Time      `json:"replayUntil,omitempty"`
}

// Validate enforces the complete persisted command and replay shape.
func (p PendingSteer) Validate() error {
	return p.validate(p.SessionID)
}

func (p PendingSteer) validate(sessionID string) error {
	if strings.TrimSpace(p.SessionID) == "" || p.SessionID != strings.TrimSpace(sessionID) {
		return errors.New("pending steer belongs to another session")
	}
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("pending steer command id is empty")
	}
	if p.StagedAt.IsZero() {
		return errors.New("pending steer staging time is empty")
	}
	if strings.TrimSpace(p.ReplayNamespace) == "" {
		if !p.ReplayUntil.IsZero() {
			return errors.New("pending steer replay deadline has no namespace")
		}
	} else if p.ReplayUntil.IsZero() || !p.ReplayUntil.After(p.StagedAt) {
		return errors.New("pending steer replay guarantee is incomplete")
	}
	return nil
}

func (p PendingSteer) clone() PendingSteer {
	p.Command = p.Command.Clone()
	return p
}

func pendingSteerEqual(left, right PendingSteer) bool {
	return left.SessionID == right.SessionID && left.Command.Equal(right.Command) &&
		left.StagedAt.Equal(right.StagedAt) && left.ReplayNamespace == right.ReplayNamespace &&
		left.ReplayUntil.Equal(right.ReplayUntil)
}

// PendingSteers returns unsettled steer commands in stable session order.
func (s *Store) PendingSteers() []PendingSteer {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := make([]PendingSteer, 0, len(s.pendingSteers))
	for _, steer := range s.pendingSteers {
		pending = append(pending, steer.clone())
	}
	slices.SortFunc(pending, func(left, right PendingSteer) int {
		return strings.Compare(left.SessionID, right.SessionID)
	})
	return pending
}

// PendingSteer returns the unsettled command for one session, when present.
func (s *Store) PendingSteer(sessionID string) (PendingSteer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[strings.TrimSpace(sessionID)]
	return pending.clone(), exists
}

// StagePendingSteer atomically transfers attachment ownership from the exact
// durable composer draft into a replayable runtime command. A crash therefore
// observes either the editable attachments or the command journal, never an
// empty gap between them.
func (s *Store) StagePendingSteer(pending PendingSteer, sourceDraft agent.Message) error {
	pending.SessionID = strings.TrimSpace(pending.SessionID)
	pending = pending.clone()
	sourceDraft = sourceDraft.Clone()
	if err := pending.validate(pending.SessionID); err != nil {
		return err
	}
	wantCommand := "/steer " + strings.TrimSpace(pending.Command.Message.Text)
	if strings.TrimSpace(sourceDraft.Text) != wantCommand {
		return errors.New("pending steer source draft does not contain the exact command")
	}
	if !slices.Equal(sourceDraft.Attachments, pending.Command.Message.Attachments) {
		return errors.New("pending steer does not own the source draft attachments")
	}
	if !messageEmpty(sourceDraft) {
		if err := sourceDraft.Validate(); err != nil {
			return fmt.Errorf("pending steer source draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingSteers[pending.SessionID]; exists {
		if pendingSteerEqual(current, pending) {
			return nil
		}
		return errors.New("another steer command is already pending")
	}
	if current, exists := s.drafts[pending.SessionID]; exists != !messageEmpty(sourceDraft) ||
		(exists && !current.Equal(sourceDraft)) {
		return errors.New("session draft changed before steer attachment transfer")
	}
	if err := s.saveSessionStateRecord(
		pending.SessionID, agent.Message{}, s.pendingRuns[pending.SessionID],
		s.pendingResumePointer(pending.SessionID), s.pendingRollbackPointer(pending.SessionID), &pending,
	); err != nil {
		return err
	}
	delete(s.drafts, pending.SessionID)
	s.pendingSteers[pending.SessionID] = pending
	return nil
}

// AcknowledgePendingSteer consumes the exact accepted command, records its
// semantic prompt history idempotently, and preserves any newer session draft.
func (s *Store) AcknowledgePendingSteer(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return errors.New("pending steer command identity changed")
	}

	message := pending.Command.Message.Clone()
	nextHistory := cloneHistory(s.history)
	historyIndex := slices.IndexFunc(nextHistory, func(entry historyEntry) bool {
		return entry.CommandID == commandID
	})
	if historyIndex >= 0 && !nextHistory[historyIndex].Equal(message) {
		return errors.New("prompt history command identity already owns another message")
	}
	if historyIndex < 0 {
		nextHistory = s.trimHistory(append(nextHistory, historyEntry{Message: message, CommandID: commandID}))
		if err := s.save("history.json", nextHistory); err != nil {
			return err
		}
		s.history = nextHistory
	}
	if err := s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	); err != nil {
		return err
	}
	delete(s.pendingSteers, sessionID)
	return nil
}

// RejectPendingSteer returns borrowed attachments after a definitive runtime
// refusal. currentDraft is an ownership precondition supplied after the
// terminal has flushed newer input, so the merge and journal retirement are
// one durable session-aggregate replacement.
func (s *Store) RejectPendingSteer(
	sessionID string,
	commandID agent.CommandID,
	currentDraft agent.Message,
) (agent.Message, error) {
	if err := commandID.Validate(); err != nil {
		return agent.Message{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	currentDraft = currentDraft.Clone()
	if !messageEmpty(currentDraft) {
		if err := currentDraft.Validate(); err != nil {
			return agent.Message{}, fmt.Errorf("current session draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return agent.Message{}, errors.New("pending steer command identity changed")
	}
	if current, present := s.drafts[sessionID]; present != !messageEmpty(currentDraft) ||
		(present && !current.Equal(currentDraft)) {
		return agent.Message{}, errors.New("session draft changed before steer rejection settlement")
	}
	recovered := MergeSteerAttachments(currentDraft, pending.Command.Message.Attachments)
	if err := s.saveSessionStateRecord(
		sessionID, recovered, s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	); err != nil {
		return agent.Message{}, err
	}
	if messageEmpty(recovered) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = recovered.Clone()
	}
	delete(s.pendingSteers, sessionID)
	return recovered, nil
}

// MergeSteerAttachments preserves newer text and attachments while returning
// each rejected attachment at most once.
func MergeSteerAttachments(current agent.Message, rejected []agent.Attachment) agent.Message {
	current = current.Clone()
	seenIDs := make(map[string]struct{}, len(current.Attachments)+len(rejected))
	seenPaths := make(map[string]struct{}, len(current.Attachments)+len(rejected))
	for _, attachment := range current.Attachments {
		seenIDs[attachment.ID] = struct{}{}
		seenPaths[attachment.Path] = struct{}{}
	}
	for _, attachment := range rejected {
		if _, duplicate := seenIDs[attachment.ID]; duplicate {
			continue
		}
		if _, duplicate := seenPaths[attachment.Path]; duplicate {
			continue
		}
		current.Attachments = append(current.Attachments, attachment)
		seenIDs[attachment.ID] = struct{}{}
		seenPaths[attachment.Path] = struct{}{}
	}
	return current
}

func (s *Store) pendingRollbackPointer(sessionID string) *PendingSessionRollback {
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists {
		return nil
	}
	cloned := pending.clone()
	return &cloned
}

func (s *Store) pendingSteerPointer(sessionID string) *PendingSteer {
	pending, exists := s.pendingSteers[sessionID]
	if !exists {
		return nil
	}
	cloned := pending.clone()
	return &cloned
}
