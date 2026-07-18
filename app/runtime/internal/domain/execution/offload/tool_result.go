// Package offload defines the durable identity and artifact record for tool
// results moved out of the model's inline context.
package offload

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
	ErrInvalidID        = errors.New("offload: invalid tool-result ID")
	ErrIdentityConflict = errors.New("offload: tool-result identity conflict")
)

// ID is the opaque identity copied from an offload preview into
// read_tool_result calls and session artifacts.
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

func (id ID) String() string { return string(id) }

// Validate accepts the uppercase unpadded base32 alphabet produced by
// crypto/rand.Text and bounds imported or model-supplied identifiers.
func (id ID) Validate() error {
	raw := string(id)
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

// ToolResultStage is the complete unbound record persisted only after its
// rendered preview has proven worth evicting from model context.
type ToolResultStage struct {
	ID        ID
	SessionID string
	ToolName  string
	Body      string
}

func (s ToolResultStage) Validate() error {
	var errs []error
	if err := s.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(s.SessionID) == "" {
		errs = append(errs, errors.New("offload: session ID is required"))
	}
	if strings.TrimSpace(s.ToolName) == "" {
		errs = append(errs, errors.New("offload: tool name is required"))
	}
	if s.Body == "" {
		errs = append(errs, errors.New("offload: body is required"))
	}
	return errors.Join(errs...)
}

// ToolResultBlob is the portable, session-owned record needed to restore both
// transcript presentation and read_tool_result behavior on another database.
type ToolResultBlob struct {
	ID        ID
	SessionID string
	ItemID    string
	ToolName  string
	Preview   string
	Body      string
	CreatedAt time.Time
}

func (b ToolResultBlob) Validate() error {
	var errs []error
	if err := b.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(b.SessionID) == "" {
		errs = append(errs, errors.New("offload: session ID is required"))
	}
	if strings.TrimSpace(b.ItemID) == "" {
		errs = append(errs, errors.New("offload: item ID is required"))
	}
	if strings.TrimSpace(b.ToolName) == "" {
		errs = append(errs, errors.New("offload: tool name is required"))
	}
	if b.Preview == "" {
		errs = append(errs, errors.New("offload: preview is required"))
	}
	if b.Body == "" {
		errs = append(errs, errors.New("offload: body is required"))
	}
	if b.CreatedAt.IsZero() {
		errs = append(errs, errors.New("offload: creation time is required"))
	}
	return errors.Join(errs...)
}
