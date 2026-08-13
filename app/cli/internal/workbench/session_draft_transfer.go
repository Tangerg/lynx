package workbench

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const sessionDraftTransferName = "session-draft-transfer.json"

// DraftTransfer is the durable ownership change for composer state that spans
// two session aggregates. Before values fence stale recovery; after values let
// Open finish either half of an interrupted transfer without guessing.
type DraftTransfer struct {
	SourceSessionID      string        `json:"sourceSessionId"`
	DestinationSessionID string        `json:"destinationSessionId"`
	SourceBefore         agent.Message `json:"sourceBefore"`
	SourceAfter          agent.Message `json:"sourceAfter"`
	DestinationBefore    agent.Message `json:"destinationBefore"`
	DestinationAfter     agent.Message `json:"destinationAfter"`
}

func (transfer DraftTransfer) clone() DraftTransfer {
	transfer.SourceBefore = transfer.SourceBefore.Clone()
	transfer.SourceAfter = transfer.SourceAfter.Clone()
	transfer.DestinationBefore = transfer.DestinationBefore.Clone()
	transfer.DestinationAfter = transfer.DestinationAfter.Clone()
	return transfer
}

func (transfer DraftTransfer) normalized() DraftTransfer {
	transfer = transfer.clone()
	transfer.SourceSessionID = strings.TrimSpace(transfer.SourceSessionID)
	transfer.DestinationSessionID = strings.TrimSpace(transfer.DestinationSessionID)
	return transfer
}

func (transfer DraftTransfer) equal(other DraftTransfer) bool {
	return transfer.SourceSessionID == other.SourceSessionID &&
		transfer.DestinationSessionID == other.DestinationSessionID &&
		transfer.SourceBefore.Equal(other.SourceBefore) &&
		transfer.SourceAfter.Equal(other.SourceAfter) &&
		transfer.DestinationBefore.Equal(other.DestinationBefore) &&
		transfer.DestinationAfter.Equal(other.DestinationAfter)
}

func (transfer DraftTransfer) validate() error {
	transfer.SourceSessionID = strings.TrimSpace(transfer.SourceSessionID)
	transfer.DestinationSessionID = strings.TrimSpace(transfer.DestinationSessionID)
	if transfer.SourceSessionID == "" || transfer.DestinationSessionID == "" {
		return errors.New("draft transfer session id is empty")
	}
	if transfer.SourceSessionID == transfer.DestinationSessionID {
		return errors.New("draft transfer source and destination are the same session")
	}
	values := []struct {
		label   string
		message agent.Message
	}{
		{label: "source before", message: transfer.SourceBefore},
		{label: "source after", message: transfer.SourceAfter},
		{label: "destination before", message: transfer.DestinationBefore},
		{label: "destination after", message: transfer.DestinationAfter},
	}
	for _, value := range values {
		if messageEmpty(value.message) {
			continue
		}
		if err := value.message.Validate(); err != nil {
			return fmt.Errorf("draft transfer %s: %w", value.label, err)
		}
	}
	if transfer.SourceBefore.Equal(transfer.SourceAfter) &&
		transfer.DestinationBefore.Equal(transfer.DestinationAfter) {
		return errors.New("draft transfer does not change authoring state")
	}
	return nil
}

func (transfer DraftTransfer) blocks(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	return sessionID == transfer.SourceSessionID || sessionID == transfer.DestinationSessionID
}

// ApplyDraftTransfer changes both session drafts under one restart-safe
// journal. If a filesystem failure leaves only one side committed, subsequent
// mutations of either aggregate fail closed until Open completes the journal.
func (s *Store) ApplyDraftTransfer(transfer DraftTransfer) error {
	transfer = transfer.normalized()
	if err := transfer.validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransfer != nil {
		if !s.draftTransfer.equal(transfer) {
			return errors.New("another session draft transfer requires recovery")
		}
		return s.completeDraftTransferLocked()
	}
	if err := transfer.expect(s.drafts); err != nil {
		return err
	}
	if err := s.save(sessionDraftTransferName, transfer); err != nil {
		return fmt.Errorf("stage session draft transfer: %w", err)
	}
	s.draftTransfer = &transfer
	return s.completeDraftTransferLocked()
}

func (transfer DraftTransfer) expect(drafts map[string]agent.Message) error {
	if !drafts[transfer.SourceSessionID].Equal(transfer.SourceBefore) {
		return errors.New("source draft changed before session transfer")
	}
	if !drafts[transfer.DestinationSessionID].Equal(transfer.DestinationBefore) {
		return errors.New("destination draft changed before session transfer")
	}
	return nil
}

func (s *Store) recoverDraftTransfer() error {
	if s.draftTransfer == nil {
		return nil
	}
	if err := s.draftTransfer.validate(); err != nil {
		return err
	}
	return s.completeDraftTransferLocked()
}

func (s *Store) completeDraftTransferLocked() error {
	transfer := s.draftTransfer
	if transfer == nil {
		return nil
	}
	if err := validateTransferSide(
		"source", s.drafts[transfer.SourceSessionID], transfer.SourceBefore, transfer.SourceAfter,
	); err != nil {
		return err
	}
	if err := validateTransferSide(
		"destination", s.drafts[transfer.DestinationSessionID],
		transfer.DestinationBefore, transfer.DestinationAfter,
	); err != nil {
		return err
	}
	// Materialize the destination first so the journal and at least one durable
	// aggregate always retain a transferred draft before its source is retired.
	if err := s.advanceDraftTransferSide(
		transfer.DestinationSessionID, transfer.DestinationBefore, transfer.DestinationAfter,
	); err != nil {
		return fmt.Errorf("save destination draft transfer: %w", err)
	}
	if err := s.advanceDraftTransferSide(
		transfer.SourceSessionID, transfer.SourceBefore, transfer.SourceAfter,
	); err != nil {
		return fmt.Errorf("save source draft transfer: %w", err)
	}
	if err := s.remove(sessionDraftTransferName); err != nil {
		return fmt.Errorf("retire session draft transfer: %w", err)
	}
	s.draftTransfer = nil
	return nil
}

func validateTransferSide(label string, current, before, after agent.Message) error {
	if current.Equal(before) || current.Equal(after) {
		return nil
	}
	return fmt.Errorf("%s draft changed while session transfer was pending", label)
}

func (s *Store) advanceDraftTransferSide(sessionID string, before, after agent.Message) error {
	current := s.drafts[sessionID]
	if current.Equal(after) {
		return nil
	}
	if !current.Equal(before) {
		return errors.New("draft no longer matches its transfer precondition")
	}
	if err := s.saveSessionStateRecordUnfenced(
		sessionID, after, s.pendingRuns[sessionID], s.pendingResumePointer(sessionID),
		s.pendingRollbackPointer(sessionID), s.pendingSteerPointer(sessionID),
	); err != nil {
		return err
	}
	if messageEmpty(after) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = after.Clone()
	}
	return nil
}

func (s *Store) draftTransferBlocks(sessionID string) bool {
	return s.draftTransfer != nil && s.draftTransfer.blocks(sessionID)
}
