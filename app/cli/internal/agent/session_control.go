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

func (scope RestoreScope) Validate() error {
	if !slices.Contains([]RestoreScope{RestoreHistory, RestoreFiles, RestoreBoth}, scope) {
		return fmt.Errorf("restore scope %q is invalid", scope)
	}
	return nil
}

// RollbackSession keeps ToRunID and every earlier root run. An empty ToRunID
// clears all history; file restoration therefore requires a concrete boundary.
type RollbackSession struct {
	SessionID string
	ToRunID   string
	Scope     RestoreScope
}

func (rollback RollbackSession) Validate() error {
	var problems []error
	if strings.TrimSpace(rollback.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := rollback.Scope.Validate(); err != nil {
		problems = append(problems, err)
	}
	if rollback.ToRunID == "" && rollback.Scope != RestoreHistory {
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

func (content InputContent) Clone() InputContent {
	content.Text = strings.Clone(content.Text)
	content.MimeType = strings.Clone(content.MimeType)
	content.Data = slices.Clone(content.Data)
	return content
}

func (content InputContent) Validate() error {
	switch content.Kind {
	case InputText:
		if strings.TrimSpace(content.Text) == "" || content.MimeType != "" || len(content.Data) != 0 {
			return errors.New("text input content is malformed")
		}
	case InputImage:
		if strings.TrimSpace(content.MimeType) == "" || len(content.Data) == 0 || content.Text != "" {
			return errors.New("image input content is malformed")
		}
	default:
		return fmt.Errorf("input content kind %q is invalid", content.Kind)
	}
	return nil
}

type DroppedRun struct {
	RunID string
	Input []InputContent
}

func (run DroppedRun) Clone() DroppedRun {
	input := run.Input
	run.Input = make([]InputContent, len(input))
	for index, content := range input {
		run.Input[index] = content.Clone()
	}
	return run
}

func (run DroppedRun) Validate() error {
	if strings.TrimSpace(run.RunID) == "" {
		return errors.New("dropped run id is empty")
	}
	for index, content := range run.Input {
		if err := content.Validate(); err != nil {
			return fmt.Errorf("dropped run input %d: %w", index+1, err)
		}
	}
	return nil
}

// OpeningText joins the first dropped root input's text blocks for restoring
// the composer. It also reports inline images that still require attachment
// materialization by the delivery layer.
func (run DroppedRun) OpeningText() (string, int) {
	parts := make([]string, 0, len(run.Input))
	images := 0
	for _, content := range run.Input {
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

func (result RollbackResult) Validate() error {
	if err := result.Session.Validate(); err != nil {
		return fmt.Errorf("rollback result: %w", err)
	}
	seen := make(map[string]struct{}, len(result.Dropped))
	for index, run := range result.Dropped {
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
func (result RollbackResult) FirstOpeningInput() (DroppedRun, bool) {
	for _, run := range result.Dropped {
		if len(run.Input) != 0 {
			return run.Clone(), true
		}
	}
	return DroppedRun{}, false
}
