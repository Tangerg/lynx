package workbench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

// SessionRollbackPhase separates a command whose runtime outcome is unknown
// from one whose authoritative session outcome was confirmed. Confirmed
// records remain in the session aggregate until activation atomically merges
// their recovered opening input into the durable draft.
type SessionRollbackPhase string

const (
	SessionRollbackPrepared  SessionRollbackPhase = "prepared"
	SessionRollbackConfirmed SessionRollbackPhase = "confirmed"
)

// PendingSessionRollback is the crash-safe rollback journal owned by one
// session aggregate. The exact before/after run projections prove history
// outcomes without replay; file-affecting commands additionally bind replay to
// one runtime idempotency store and its advertised retention window.
type PendingSessionRollback struct {
	Phase           SessionRollbackPhase `json:"phase"`
	CommandID       agent.CommandID      `json:"commandId"`
	SessionID       string               `json:"sessionId"`
	ToRunID         string               `json:"toRunId,omitempty"`
	Scope           agent.RestoreScope   `json:"scope"`
	BeforeRevision  uint64               `json:"beforeRevision"`
	BeforeRunIDs    []string             `json:"beforeRunIds"`
	AfterRunIDs     []string             `json:"afterRunIds"`
	OpeningText     string               `json:"openingText,omitempty"`
	OpeningImages   int                  `json:"openingImages,omitempty"`
	StagedAt        time.Time            `json:"stagedAt"`
	ReplayNamespace string               `json:"replayNamespace,omitempty"`
	ReplayUntil     time.Time            `json:"replayUntil,omitempty"`
}

func (p PendingSessionRollback) Validate() error {
	request := p.Request()
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CommandID == "" {
		return errors.New("session rollback command id is empty")
	}
	if p.Phase != SessionRollbackPrepared && p.Phase != SessionRollbackConfirmed {
		return fmt.Errorf("session rollback phase %q is invalid", p.Phase)
	}
	if p.StagedAt.IsZero() {
		return errors.New("session rollback staging time is empty")
	}
	if p.OpeningImages < 0 {
		return errors.New("session rollback opening image count is negative")
	}
	if err := validateRunIDs("session rollback before projection", p.BeforeRunIDs); err != nil {
		return err
	}
	if err := validateRunIDs("session rollback after projection", p.AfterRunIDs); err != nil {
		return err
	}
	if p.Scope == agent.RestoreFiles {
		if !slices.Equal(p.BeforeRunIDs, p.AfterRunIDs) {
			return errors.New("files-only rollback changes the history projection")
		}
	} else if len(p.AfterRunIDs) > len(p.BeforeRunIDs) ||
		!slices.Equal(p.AfterRunIDs, p.BeforeRunIDs[:len(p.AfterRunIDs)]) {
		return errors.New("session rollback after projection is not a prefix of its before projection")
	}
	if p.ToRunID == "" {
		if p.Scope != agent.RestoreFiles && len(p.AfterRunIDs) != 0 {
			return errors.New("full history rollback retains a run projection")
		}
	} else if p.Scope == agent.RestoreFiles {
		if !slices.Contains(p.BeforeRunIDs, p.ToRunID) {
			return errors.New("file rollback boundary is absent from its history projection")
		}
	} else if !slices.Contains(p.AfterRunIDs, p.ToRunID) {
		return errors.New("session rollback boundary is absent from its after projection")
	}
	if p.Scope != agent.RestoreHistory {
		if strings.TrimSpace(p.ReplayNamespace) == "" || p.ReplayUntil.IsZero() ||
			!p.ReplayUntil.After(p.StagedAt) {
			return errors.New("file rollback replay guarantee is incomplete")
		}
	}
	return nil
}

// Request reconstructs the exact runtime mutation owned by this journal.
func (p PendingSessionRollback) Request() agent.RollbackSession {
	return agent.RollbackSession{
		CommandID: p.CommandID, SessionID: p.SessionID,
		ToRunID: p.ToRunID, Scope: p.Scope,
	}
}

func (p PendingSessionRollback) clone() PendingSessionRollback {
	p.BeforeRunIDs = slices.Clone(p.BeforeRunIDs)
	p.AfterRunIDs = slices.Clone(p.AfterRunIDs)
	return p
}

func pendingSessionRollbackEqual(left, right PendingSessionRollback) bool {
	return left.Phase == right.Phase && left.CommandID == right.CommandID &&
		left.SessionID == right.SessionID && left.ToRunID == right.ToRunID && left.Scope == right.Scope &&
		left.BeforeRevision == right.BeforeRevision && slices.Equal(left.BeforeRunIDs, right.BeforeRunIDs) &&
		slices.Equal(left.AfterRunIDs, right.AfterRunIDs) && left.OpeningText == right.OpeningText &&
		left.OpeningImages == right.OpeningImages && left.StagedAt.Equal(right.StagedAt) &&
		left.ReplayNamespace == right.ReplayNamespace && left.ReplayUntil.Equal(right.ReplayUntil)
}

func validateRunIDs(name string, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("%s item %d is empty", name, index+1)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%s repeats %q", name, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// SessionRollbackRecovery is the one-time local authoring result consumed
// when a session with a confirmed rollback becomes active.
type SessionRollbackRecovery struct {
	Draft         agent.Message
	DroppedCount  int
	OpeningImages int
	Merged        bool
}

// PendingSessionRollbacks returns rollback journals in stable session order.
func (s *Store) PendingSessionRollbacks() []PendingSessionRollback {
	s.mu.Lock()
	defer s.mu.Unlock()
	rollbacks := make([]PendingSessionRollback, 0, len(s.pendingRollbacks))
	for _, pending := range s.pendingRollbacks {
		rollbacks = append(rollbacks, pending.clone())
	}
	slices.SortFunc(rollbacks, func(left, right PendingSessionRollback) int {
		return strings.Compare(left.SessionID, right.SessionID)
	})
	return rollbacks
}

// PendingSessionRollback returns the journal for one session, when present.
func (s *Store) PendingSessionRollback(sessionID string) (PendingSessionRollback, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[strings.TrimSpace(sessionID)]
	return pending.clone(), exists
}

// StageSessionRollback durably owns a rollback before it can leave the
// process. Repeating the exact journal is idempotent; another command cannot
// replace an outcome whose acknowledgement is still unknown.
func (s *Store) StageSessionRollback(pending PendingSessionRollback) error {
	pending.SessionID = strings.TrimSpace(pending.SessionID)
	if err := pending.Validate(); err != nil {
		return err
	}
	pending = pending.clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingRollbacks[pending.SessionID]; exists {
		if pendingSessionRollbackEqual(current, pending) {
			return nil
		}
		return errors.New("another session rollback is already pending")
	}
	if err := s.saveSessionStateRecord(
		pending.SessionID, s.drafts[pending.SessionID], s.pendingRuns[pending.SessionID],
		s.pendingResumePointer(pending.SessionID), &pending, s.pendingSteerPointer(pending.SessionID),
	); err != nil {
		return err
	}
	s.pendingRollbacks[pending.SessionID] = pending
	return nil
}

// ConfirmSessionRollback upgrades the exact prepared command to a durable
// local recovery record. Draft materialization remains a separate activation
// transition so a non-active session cannot replace the visible composer.
func (s *Store) ConfirmSessionRollback(sessionID string, commandID agent.CommandID) error {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists || pending.CommandID != commandID {
		return errors.New("session rollback journal does not match the confirmed command")
	}
	if pending.Phase == SessionRollbackConfirmed {
		return nil
	}
	pending.Phase = SessionRollbackConfirmed
	if err := s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), &pending, s.pendingSteerPointer(sessionID),
	); err != nil {
		return err
	}
	s.pendingRollbacks[sessionID] = pending
	return nil
}

// RejectSessionRollback retires only the exact prepared command after a
// definitive runtime refusal.
func (s *Store) RejectSessionRollback(sessionID string, commandID agent.CommandID) error {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists {
		return nil
	}
	if pending.Phase != SessionRollbackPrepared || pending.CommandID != commandID {
		return errors.New("session rollback journal does not match the rejected command")
	}
	if err := s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), nil, s.pendingSteerPointer(sessionID),
	); err != nil {
		return err
	}
	delete(s.pendingRollbacks, sessionID)
	return nil
}

// ConsumeConfirmedSessionRollback atomically merges the recovered opening
// input with any newer draft and retires the journal. A newer user draft is
// appended after the restored opening text so neither value is discarded.
func (s *Store) ConsumeConfirmedSessionRollback(sessionID string) (SessionRollbackRecovery, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists || pending.Phase != SessionRollbackConfirmed {
		return SessionRollbackRecovery{}, false, nil
	}
	draft, merged := MergeSessionRollbackDraft(s.drafts[sessionID], pending)
	if err := s.saveSessionStateRecord(
		sessionID, draft, s.pendingRuns[sessionID], s.pendingResumePointer(sessionID), nil,
		s.pendingSteerPointer(sessionID),
	); err != nil {
		return SessionRollbackRecovery{}, false, err
	}
	if messageEmpty(draft) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = draft.Clone()
	}
	delete(s.pendingRollbacks, sessionID)
	return SessionRollbackRecovery{
		Draft: draft.Clone(), DroppedCount: len(pending.BeforeRunIDs) - len(pending.AfterRunIDs),
		OpeningImages: pending.OpeningImages, Merged: merged,
	}, true, nil
}

func (s *Store) pendingResumePointer(sessionID string) *PendingResume {
	pending, exists := s.pendingResumes[sessionID]
	if !exists {
		return nil
	}
	return &pending
}

// MergeSessionRollbackDraft restores one journal's opening text without
// discarding a newer draft authored while the runtime mutation was settling.
func MergeSessionRollbackDraft(current agent.Message, pending PendingSessionRollback) (agent.Message, bool) {
	recovered := agent.Message{Text: pending.OpeningText}
	current = current.Clone()
	recovered = recovered.Clone()
	if messageEmpty(recovered) {
		return current, false
	}
	if messageEmpty(current) {
		return recovered, false
	}
	if current.Equal(recovered) ||
		(strings.TrimSpace(recovered.Text) != "" && strings.HasPrefix(current.Text, recovered.Text)) {
		return current, false
	}
	if strings.TrimSpace(recovered.Text) != "" {
		if strings.TrimSpace(current.Text) == "" {
			current.Text = recovered.Text
		} else {
			current.Text = recovered.Text + "\n\n" + current.Text
		}
	}
	return current, true
}
