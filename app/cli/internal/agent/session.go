package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Session is one durable conversation.
type Session struct {
	ID        string
	Title     string
	Workspace string
	UpdatedAt time.Time
	Revision  int64
}

// Validate checks session metadata returned by a runtime before an adapter uses it.
func (s Session) Validate() error {
	var problems []error
	if strings.TrimSpace(s.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(s.Workspace) == "" {
		problems = append(problems, errors.New("workspace is empty"))
	}
	if s.Revision < 0 {
		problems = append(problems, errors.New("revision is negative"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	return nil
}

// SessionQuery requests one stable page, newest first.
type SessionQuery struct {
	Cursor    string
	Limit     int
	Search    string
	Workspace string
}

// SessionPage is a page plus an opaque cursor for the next page.
type SessionPage struct {
	Items      []Session
	NextCursor string
}

// Validate checks every listed session and rejects duplicate identities.
func (p SessionPage) Validate() error {
	seen := make(map[string]struct{}, len(p.Items))
	for index, session := range p.Items {
		if err := session.Validate(); err != nil {
			return fmt.Errorf("session page item %d: %w", index+1, err)
		}
		if _, duplicate := seen[session.ID]; duplicate {
			return fmt.Errorf("session page repeats id %q", session.ID)
		}
		seen[session.ID] = struct{}{}
	}
	return nil
}

// SessionSnapshot is sufficient to rebuild the terminal without a second
// transcript endpoint. Events are ordered from cursor one through Cursor.
type SessionSnapshot struct {
	Session Session
	Events  []Envelope
	Cursor  Cursor
	Active  *Run
}

// Validate proves that metadata, replay history, aggregate phase, and active
// run describe the same authoritative session state.
func (s SessionSnapshot) Validate() error {
	_, err := foldSnapshot(s)
	return err
}

// RestoreSnapshot atomically replaces the aggregate with a validated snapshot.
func (c *Conversation) RestoreSnapshot(snapshot SessionSnapshot) error {
	next, err := foldSnapshot(snapshot)
	if err != nil {
		return err
	}
	*c = *next
	return nil
}

func foldSnapshot(snapshot SessionSnapshot) (*Conversation, error) {
	if err := validateSnapshotHeader(snapshot); err != nil {
		return nil, err
	}
	conversation := NewConversation()
	if err := foldSnapshotEvents(snapshot, conversation); err != nil {
		return nil, err
	}
	if conversation.Cursor() != snapshot.Cursor {
		return nil, fmt.Errorf("session snapshot: folded cursor %d does not match %d", conversation.Cursor(), snapshot.Cursor)
	}
	if err := validateActiveRun(snapshot, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func validateSnapshotHeader(snapshot SessionSnapshot) error {
	if err := snapshot.Session.Validate(); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	switch {
	case snapshot.Cursor == 0 && len(snapshot.Events) != 0:
		return errors.New("session snapshot: zero cursor carries events")
	case snapshot.Cursor != 0 && (len(snapshot.Events) == 0 || snapshot.Events[len(snapshot.Events)-1].Cursor != snapshot.Cursor):
		return fmt.Errorf("session snapshot: cursor %d does not match event history", snapshot.Cursor)
	default:
		return nil
	}
}

func foldSnapshotEvents(snapshot SessionSnapshot, conversation *Conversation) error {
	for _, envelope := range snapshot.Events {
		if envelope.SessionID != snapshot.Session.ID {
			return fmt.Errorf("session snapshot: event %s belongs to session %s", envelope.ID, envelope.SessionID)
		}
		if _, err := conversation.ApplyEnvelope(envelope); err != nil {
			return fmt.Errorf("session snapshot: cursor %d: %w", envelope.Cursor, err)
		}
	}
	return nil
}

func validateActiveRun(snapshot SessionSnapshot, conversation *Conversation) error {
	if snapshot.Active == nil {
		if conversation.Phase() != ConversationIdle {
			return errors.New("session snapshot: busy conversation has no active run")
		}
		return nil
	}
	active := *snapshot.Active
	if err := validateActiveProjection(active, snapshot.Session.ID, conversation); err != nil {
		return err
	}
	return validateActiveStart(active, snapshot.Events)
}

func validateActiveProjection(active Run, sessionID string, conversation *Conversation) error {
	if err := active.Validate(); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	if active.Status == RunComplete {
		return errors.New("session snapshot: completed run is marked active")
	}
	if active.SessionID != sessionID {
		return fmt.Errorf("session snapshot: active run belongs to session %s", active.SessionID)
	}
	if conversation.RunID() != active.ID {
		return fmt.Errorf("session snapshot: aggregate run %s does not match active run %s", conversation.RunID(), active.ID)
	}
	wantPhase := ConversationRunning
	if active.Status == RunWaiting {
		wantPhase = ConversationWaiting
	}
	if conversation.Phase() != wantPhase {
		return fmt.Errorf("session snapshot: active run status %s conflicts with conversation phase %d", active.Status, conversation.Phase())
	}
	return nil
}

func validateActiveStart(active Run, events []Envelope) error {
	if active.StartedAfter == ^Cursor(0) {
		return errors.New("session snapshot: active run start cursor overflows")
	}
	startedAt := active.StartedAfter + 1
	for _, envelope := range events {
		if envelope.Cursor != startedAt || envelope.RunID != active.ID {
			continue
		}
		started, ok := envelope.Event.(RunStarted)
		if ok && started.RunID == active.ID {
			return nil
		}
		break
	}
	return fmt.Errorf("session snapshot: active run %s has no start event at cursor %d", active.ID, startedAt)
}

type CreateSession struct {
	Title     string
	Workspace string
}

// UpdateSession uses optimistic concurrency. Zero Revision disables the check
// for explicitly unconditional administrative commands.
type UpdateSession struct {
	SessionID string
	Title     string
	Revision  int64
}

// ForkSession creates a new session from the source timeline through At. At
// must end at a settled run boundary; zero means the source's latest cursor.
type ForkSession struct {
	SessionID string
	At        Cursor
	Title     string
}

type DeleteSession struct {
	SessionID string
	Revision  int64
}
