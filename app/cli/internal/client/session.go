package client

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
	if strings.TrimSpace(snapshot.Session.ID) == "" {
		return nil, errors.New("session snapshot: session id is empty")
	}
	if strings.TrimSpace(snapshot.Session.Workspace) == "" {
		return nil, errors.New("session snapshot: workspace is empty")
	}
	if snapshot.Cursor == 0 && len(snapshot.Events) != 0 {
		return nil, errors.New("session snapshot: zero cursor carries events")
	}
	if snapshot.Cursor != 0 && (len(snapshot.Events) == 0 || snapshot.Events[len(snapshot.Events)-1].Cursor != snapshot.Cursor) {
		return nil, fmt.Errorf("session snapshot: cursor %d does not match event history", snapshot.Cursor)
	}

	conversation := NewConversation()
	for _, envelope := range snapshot.Events {
		if envelope.SessionID != snapshot.Session.ID {
			return nil, fmt.Errorf("session snapshot: event %s belongs to session %s", envelope.ID, envelope.SessionID)
		}
		if _, err := conversation.ApplyEnvelope(envelope); err != nil {
			return nil, fmt.Errorf("session snapshot: cursor %d: %w", envelope.Cursor, err)
		}
	}
	if conversation.Cursor() != snapshot.Cursor {
		return nil, fmt.Errorf("session snapshot: folded cursor %d does not match %d", conversation.Cursor(), snapshot.Cursor)
	}
	if err := validateActiveRun(snapshot, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func validateActiveRun(snapshot SessionSnapshot, conversation *Conversation) error {
	if snapshot.Active == nil {
		if conversation.Phase() != Idle {
			return errors.New("session snapshot: busy conversation has no active run")
		}
		return nil
	}
	active := *snapshot.Active
	if err := active.Validate(); err != nil {
		return fmt.Errorf("session snapshot: %w", err)
	}
	if active.Status == RunComplete {
		return errors.New("session snapshot: completed run is marked active")
	}
	if active.SessionID != snapshot.Session.ID {
		return fmt.Errorf("session snapshot: active run belongs to session %s", active.SessionID)
	}
	if conversation.RunID() != active.ID {
		return fmt.Errorf("session snapshot: aggregate run %s does not match active run %s", conversation.RunID(), active.ID)
	}
	wantPhase := Running
	if active.Status == RunWaiting {
		wantPhase = Waiting
	}
	if conversation.Phase() != wantPhase {
		return fmt.Errorf("session snapshot: active run status %s conflicts with conversation phase %d", active.Status, conversation.Phase())
	}
	if active.StartedAfter == ^Cursor(0) {
		return errors.New("session snapshot: active run start cursor overflows")
	}
	startedAt := active.StartedAfter + 1
	found := false
	for _, envelope := range snapshot.Events {
		if envelope.Cursor != startedAt || envelope.RunID != active.ID {
			continue
		}
		started, ok := envelope.Event.(RunStarted)
		found = ok && started.RunID == active.ID
		break
	}
	if !found {
		return fmt.Errorf("session snapshot: active run %s has no start event at cursor %d", active.ID, startedAt)
	}
	return nil
}

type NewSession struct {
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
