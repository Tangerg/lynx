package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RestoreScope is the portion of a session rollback rewinds.
type RestoreScope string

const (
	RestoreHistory RestoreScope = "history"
	RestoreFiles   RestoreScope = "files"
	RestoreBoth    RestoreScope = "both"
)

func (r RestoreScope) Validate() error {
	if !slices.Contains([]RestoreScope{RestoreHistory, RestoreFiles, RestoreBoth}, r) {
		return fmt.Errorf("restore scope %q is invalid", r)
	}
	return nil
}

// RollbackSession keeps ToRunID and every earlier root run. An empty ToRunID
// clears all history; file restoration therefore requires a concrete boundary.
type RollbackSession struct {
	CommandID CommandID
	SessionID string
	ToRunID   string
	Scope     RestoreScope
}

func (r RollbackSession) Validate() error {
	var problems []error
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(r.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := r.Scope.Validate(); err != nil {
		problems = append(problems, err)
	}
	if r.ToRunID == "" && r.Scope != RestoreHistory {
		problems = append(problems, errors.New("file restoration requires a run boundary"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("rollback session: %w", err)
	}
	return nil
}

type InputContentKind string

const (
	InputText  InputContentKind = "text"
	InputImage InputContentKind = "image"
)

// InputContent preserves a dropped run's opening input without pretending an
// inline runtime image is already a local authoring attachment.
type InputContent struct {
	Kind     InputContentKind
	Text     string
	MimeType string
	Data     []byte
}

func (i InputContent) Clone() InputContent {
	i.Text = strings.Clone(i.Text)
	i.MimeType = strings.Clone(i.MimeType)
	i.Data = slices.Clone(i.Data)
	return i
}

func (i InputContent) Validate() error {
	switch i.Kind {
	case InputText:
		if strings.TrimSpace(i.Text) == "" || i.MimeType != "" || len(i.Data) != 0 {
			return errors.New("text input content is malformed")
		}
	case InputImage:
		if strings.TrimSpace(i.MimeType) == "" || len(i.Data) == 0 || i.Text != "" {
			return errors.New("image input content is malformed")
		}
	default:
		return fmt.Errorf("input content kind %q is invalid", i.Kind)
	}
	return nil
}

type DroppedRun struct {
	RunID string
	Input []InputContent
}

func (d DroppedRun) Clone() DroppedRun {
	input := d.Input
	d.Input = make([]InputContent, len(input))
	for index, content := range input {
		d.Input[index] = content.Clone()
	}
	return d
}

func (d DroppedRun) Validate() error {
	if strings.TrimSpace(d.RunID) == "" {
		return errors.New("dropped run id is empty")
	}
	for index, content := range d.Input {
		if err := content.Validate(); err != nil {
			return fmt.Errorf("dropped run input %d: %w", index+1, err)
		}
	}
	return nil
}

// OpeningText joins the first dropped root input's text blocks for restoring
// the composer. It also reports inline images that still require attachment
// materialization by the delivery layer.
func (d DroppedRun) OpeningText() (string, int) {
	parts := make([]string, 0, len(d.Input))
	images := 0
	for _, content := range d.Input {
		switch content.Kind {
		case InputText:
			parts = append(parts, content.Text)
		case InputImage:
			images++
		}
	}
	return strings.Join(parts, "\n\n"), images
}

type RollbackResult struct {
	Session Session
	Dropped []DroppedRun
}

func (r RollbackResult) Validate() error {
	if err := r.Session.Validate(); err != nil {
		return fmt.Errorf("rollback result: %w", err)
	}
	seen := make(map[string]struct{}, len(r.Dropped))
	for index, run := range r.Dropped {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("rollback result dropped run %d: %w", index+1, err)
		}
		if _, duplicate := seen[run.RunID]; duplicate {
			return fmt.Errorf("rollback result repeats run %q", run.RunID)
		}
		seen[run.RunID] = struct{}{}
	}
	return nil
}

// FirstOpeningInput returns the earliest dropped root input. Child and
// continuation runs carry no opening input and are skipped.
func (r RollbackResult) FirstOpeningInput() (DroppedRun, bool) {
	for _, run := range r.Dropped {
		if len(run.Input) != 0 {
			return run.Clone(), true
		}
	}
	return DroppedRun{}, false
}
