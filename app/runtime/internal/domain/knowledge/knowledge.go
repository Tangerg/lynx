// Package knowledge defines Lyra's human-authored long-term knowledge: the
// user-editable LYRA.md cascade. Agent-maintained memory (the mined fact ledger
// and its curated items) is a separate bounded context — see package
// agentmemory. Prompt composition remains outside both storage domains.
package knowledge

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrRevisionRequired reports a knowledge mutation without the revision it read.
	ErrRevisionRequired = errors.New("knowledge: expected revision is required")
	// ErrRevisionConflict reports that the document changed after it was read.
	ErrRevisionConflict = errors.New("knowledge: revision conflict")
	// ErrPathOutsideScope reports a document that does not belong to the selected scope.
	ErrPathOutsideScope = errors.New("knowledge: path outside scope")
	// ErrDocumentTooLarge reports authored knowledge that cannot enter the
	// complete management and prompt projections.
	ErrDocumentTooLarge = errors.New("knowledge: document too large")
)

// MaxDocumentBytes is the complete encoded size admitted for one LYRA.md.
const MaxDocumentBytes int64 = 1 << 20

// ValidateDocumentSize enforces the single resource envelope shared by
// command admission and filesystem reads.
func ValidateDocumentSize(size int64) error {
	if size < 0 || size > MaxDocumentBytes {
		return fmt.Errorf(
			"%w: %d bytes, maximum %d",
			ErrDocumentTooLarge,
			size,
			MaxDocumentBytes,
		)
	}
	return nil
}

// ValidateDocument validates an already-owned knowledge document.
func ValidateDocument(content string) error {
	return ValidateDocumentSize(int64(len(content)))
}

// Scope selects one location in the human-authored LYRA.md cascade. The three
// locations are distinct even when a workspace happens to be its project root:
// callers may address that file through either semantic scope, while cascade
// readers de-duplicate the shared physical document.
type Scope string

const (
	// ScopeCWD is the LYRA.md at the workspace resource root.
	ScopeCWD Scope = "cwd"
	// ScopeProjectRoot is the LYRA.md at the nearest project-discovery root.
	ScopeProjectRoot Scope = "projectRoot"
	// ScopeHome is the cross-project LYRA.md in the Runtime's user data root.
	ScopeHome Scope = "home"
)

// Valid reports whether s names one of the three human-authored knowledge
// locations.
func (s Scope) Valid() bool {
	return s == ScopeCWD || s == ScopeProjectRoot || s == ScopeHome
}

// Validate rejects a value that cannot identify a LYRA.md location.
func (s Scope) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("knowledge: invalid scope %q", s)
	}
	return nil
}

func (s Scope) String() string { return string(s) }

// Entry is one human-authored knowledge document. Content is its verbatim
// Markdown; UpdatedAt records the document's last modification.
type Entry struct {
	Scope     Scope
	Path      string
	Content   string
	Revision  string
	UpdatedAt time.Time
}
