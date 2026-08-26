// Package toolresult defines the durable identity and artifact record for tool
// results moved out of the model's inline context.
package toolresult

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	minIDLength = 2
	maxIDLength = 64
)

var (
	ErrInvalidID        = errors.New("toolresult: invalid tool-result ID")
	ErrIdentityConflict = errors.New("toolresult: tool-result identity conflict")
)

// ID is the opaque identity copied from an offload preview into
// later result reads and portable session exports.
type ID string

// NewID returns a new unguessable tool-result identity.
func NewID() ID { return ID(rand.Text()) }

// ParseID validates raw before admitting it as an offloaded-result identity.
func ParseID(raw string) (ID, error) {
	id := ID(raw)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

func (i ID) String() string { return string(i) }

// Validate accepts the uppercase unpadded base32 alphabet produced by
// crypto/rand.Text and bounds imported or model-supplied identifiers.
func (i ID) Validate() error {
	raw := string(i)
	if len(raw) < minIDLength || len(raw) > maxIDLength {
		return fmt.Errorf("%w: length must be between %d and %d characters", ErrInvalidID, minIDLength, maxIDLength)
	}
	for _, char := range raw {
		if (char < 'A' || char > 'Z') && (char < '2' || char > '7') {
			return fmt.Errorf("%w: %q is not uppercase base32", ErrInvalidID, raw)
		}
	}
	return nil
}

// Ref is the typed link carried with a transcript item after its full result
// has been moved to durable blob storage.
type Ref struct {
	ID ID
}

func (r Ref) Validate() error { return r.ID.Validate() }

// Stage is the complete unbound record persisted only after its
// rendered preview has proven worth evicting from model context.
type Stage struct {
	ID        ID
	SessionID string
	ToolName  string
	Body      string
}

func (s Stage) Validate() error {
	var errs []error
	if err := s.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(s.SessionID) == "" {
		errs = append(errs, errors.New("toolresult: session ID is required"))
	}
	if strings.TrimSpace(s.ToolName) == "" {
		errs = append(errs, errors.New("toolresult: tool name is required"))
	}
	if s.Body == "" {
		errs = append(errs, errors.New("toolresult: body is required"))
	}
	return errors.Join(errs...)
}

// Blob is the portable, session-owned record needed to restore both
// transcript reconstruction and deferred result reads on another database.
type Blob struct {
	ID        ID
	SessionID string
	ItemID    string
	ToolName  string
	Preview   string
	Body      string
	CreatedAt time.Time
}

func (b Blob) Validate() error {
	var errs []error
	if err := b.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(b.SessionID) == "" {
		errs = append(errs, errors.New("toolresult: session ID is required"))
	}
	if strings.TrimSpace(b.ItemID) == "" {
		errs = append(errs, errors.New("toolresult: item ID is required"))
	}
	if strings.TrimSpace(b.ToolName) == "" {
		errs = append(errs, errors.New("toolresult: tool name is required"))
	}
	if b.Preview == "" {
		errs = append(errs, errors.New("toolresult: preview is required"))
	}
	if b.Body == "" {
		errs = append(errs, errors.New("toolresult: body is required"))
	}
	if b.CreatedAt.IsZero() {
		errs = append(errs, errors.New("toolresult: creation time is required"))
	}
	return errors.Join(errs...)
}
