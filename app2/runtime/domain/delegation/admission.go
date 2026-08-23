// Package delegation owns the durable handshake between a model-authored
// delegate request and the product Run that may be created for it.
package delegation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidAdmission  = errors.New("delegation: invalid admission")
	ErrAdmissionConflict = errors.New("delegation: admission identity conflict")
	ErrNotFound          = errors.New("delegation: admission not found")
)

type Status string

const (
	Pending Status = "pending"
	Started Status = "started"
	Aborted Status = "aborted"
)

// Admission is one exact, replay-safe child-start reservation. MemberID and
// ParentMemberID are private executor identities; RunID and SegmentID are
// product identities allocated before the Framework publishes a child. A
// pending value is not a Run and is never projected on the Lyra wire.
type Admission struct {
	MemberID, ParentMemberID, ChildKey string
	RunID, SegmentID                   string
	SessionID, ParentRunID, RootRunID  string
	SpawnedByItemID                    string
	Provider, Model                    string
	Summary, Instructions              string
	Status                             Status
	Failure                            string
	StartedAt, UpdatedAt               time.Time
}

type Reserve struct {
	MemberID, ParentMemberID, ChildKey string
	RunID, SegmentID                   string
	SessionID, ParentRunID, RootRunID  string
	SpawnedByItemID                    string
	Provider, Model                    string
	Summary, Instructions              string
	StartedAt                          time.Time
}

func New(command Reserve) (Admission, error) {
	value := Admission{
		MemberID: command.MemberID, ParentMemberID: command.ParentMemberID,
		ChildKey: command.ChildKey, RunID: command.RunID, SegmentID: command.SegmentID,
		SessionID: command.SessionID, ParentRunID: command.ParentRunID,
		RootRunID: command.RootRunID, SpawnedByItemID: command.SpawnedByItemID,
		Provider: command.Provider, Model: command.Model,
		Summary: strings.TrimSpace(command.Summary), Instructions: strings.TrimSpace(command.Instructions),
		Status: Pending, StartedAt: command.StartedAt.UTC(), UpdatedAt: command.StartedAt.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Admission{}, err
	}
	return value, nil
}

func Rehydrate(value Admission) (Admission, error) {
	value.StartedAt = value.StartedAt.UTC()
	value.UpdatedAt = value.UpdatedAt.UTC()
	if err := value.Validate(); err != nil {
		return Admission{}, err
	}
	return value, nil
}

// SameReservation reports whether a replay names the exact immutable request.
// Status, Failure and UpdatedAt are intentionally excluded: they are the
// aggregate's evolving conclusion, not caller-controlled identity.
func (value Admission) SameReservation(other Admission) bool {
	return value.MemberID == other.MemberID && value.ParentMemberID == other.ParentMemberID &&
		value.ChildKey == other.ChildKey && value.RunID == other.RunID && value.SegmentID == other.SegmentID &&
		value.SessionID == other.SessionID && value.ParentRunID == other.ParentRunID &&
		value.RootRunID == other.RootRunID && value.SpawnedByItemID == other.SpawnedByItemID &&
		value.Provider == other.Provider && value.Model == other.Model &&
		value.Summary == other.Summary && value.Instructions == other.Instructions &&
		value.StartedAt.Equal(other.StartedAt)
}

func (value *Admission) MarkStarted(now time.Time) error {
	if value.Status != Pending {
		if value.Status == Started {
			return nil
		}
		return fmt.Errorf("%w: aborted admission cannot start", ErrAdmissionConflict)
	}
	value.Status = Started
	value.UpdatedAt = now.UTC()
	return value.Validate()
}

func (value *Admission) MarkAborted(failure string, now time.Time) error {
	failure = strings.TrimSpace(failure)
	if value.Status != Pending {
		if value.Status == Aborted && value.Failure == failure {
			return nil
		}
		return fmt.Errorf("%w: concluded admission changed outcome", ErrAdmissionConflict)
	}
	value.Status = Aborted
	value.Failure = failure
	value.UpdatedAt = now.UTC()
	return value.Validate()
}

func (value Admission) Validate() error {
	switch {
	case strings.TrimSpace(value.MemberID) == "" || strings.TrimSpace(value.ParentMemberID) == "" || strings.TrimSpace(value.ChildKey) == "":
		return fmt.Errorf("%w: executor lineage is required", ErrInvalidAdmission)
	case value.MemberID == value.ParentMemberID:
		return fmt.Errorf("%w: child and parent executor identities match", ErrInvalidAdmission)
	case strings.TrimSpace(value.RunID) == "" || strings.TrimSpace(value.SegmentID) == "" ||
		strings.TrimSpace(value.SessionID) == "" || strings.TrimSpace(value.ParentRunID) == "" ||
		strings.TrimSpace(value.RootRunID) == "" || strings.TrimSpace(value.SpawnedByItemID) == "":
		return fmt.Errorf("%w: product lineage is required", ErrInvalidAdmission)
	case value.RunID == value.ParentRunID || value.RunID == value.RootRunID:
		return fmt.Errorf("%w: child Run identity overlaps its ancestors", ErrInvalidAdmission)
	case strings.TrimSpace(value.Provider) == "" || strings.TrimSpace(value.Model) == "":
		return fmt.Errorf("%w: model selection is required", ErrInvalidAdmission)
	case value.Summary == "" || value.Instructions == "" ||
		strings.TrimSpace(value.Summary) != value.Summary || strings.TrimSpace(value.Instructions) != value.Instructions:
		return fmt.Errorf("%w: delegated task is required", ErrInvalidAdmission)
	case len(value.Summary) > 512 || len(value.Instructions) > 64<<10:
		return fmt.Errorf("%w: delegated task exceeds its bound", ErrInvalidAdmission)
	case value.Status != Pending && value.Status != Started && value.Status != Aborted:
		return fmt.Errorf("%w: status %q", ErrInvalidAdmission, value.Status)
	case value.Status == Aborted && value.Failure == "":
		return fmt.Errorf("%w: aborted admission has no failure", ErrInvalidAdmission)
	case value.Status != Aborted && value.Failure != "":
		return fmt.Errorf("%w: non-aborted admission carries failure", ErrInvalidAdmission)
	case value.StartedAt.IsZero() || value.UpdatedAt.IsZero() || value.UpdatedAt.Before(value.StartedAt):
		return fmt.Errorf("%w: timestamps are invalid", ErrInvalidAdmission)
	default:
		return nil
	}
}
