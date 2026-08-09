package client

import "time"

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

// ForkSession creates a new session from the source timeline through At.
// Zero At means the source's latest cursor.
type ForkSession struct {
	SessionID string
	At        Cursor
	Title     string
}

type DeleteSession struct {
	SessionID string
	Revision  int64
}
