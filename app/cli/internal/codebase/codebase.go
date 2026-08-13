// Package codebase defines the CLI's semantic-code-index projection and use
// cases. Index construction and embedding remain runtime responsibilities.
package codebase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type State string

const (
	NotIndexed State = "none"
	Indexing   State = "indexing"
	Ready      State = "ready"
	Failed     State = "error"
)

func (state State) Validate() error {
	switch state {
	case NotIndexed, Indexing, Ready, Failed:
		return nil
	default:
		return fmt.Errorf("codebase state %q is invalid", state)
	}
}

type Status struct {
	State       State
	ModelID     string
	FileCount   int
	ChunkCount  int
	IndexedAt   *time.Time
	Truncated   bool
	OperationID string
}

func (status Status) Validate() error {
	if err := status.State.Validate(); err != nil {
		return err
	}
	if status.FileCount < 0 || status.ChunkCount < 0 {
		return errors.New("codebase status has negative counts")
	}
	if status.IndexedAt != nil && status.IndexedAt.IsZero() {
		return errors.New("codebase status has a zero index time")
	}
	return nil
}

// ValidateReindexAcknowledgement binds an authoritative status read to the
// operation returned by Reindex whenever the status still reports an active
// operation. A fast rebuild may finish before the read and legitimately omit
// OperationID.
func (status Status) ValidateReindexAcknowledgement(operation ReindexOperation) error {
	if err := operation.Validate(); err != nil {
		return err
	}
	if err := status.Validate(); err != nil {
		return err
	}
	if status.OperationID != "" && status.OperationID != operation.ID {
		return fmt.Errorf("codebase status belongs to operation %q, want %q", status.OperationID, operation.ID)
	}
	return nil
}

type Query struct {
	Workspace string
	Text      string
	Limit     int
}

func (query Query) Validate() error {
	if strings.TrimSpace(query.Workspace) == "" {
		return errors.New("codebase query workspace is empty")
	}
	if strings.TrimSpace(query.Text) == "" {
		return errors.New("codebase query text is empty")
	}
	if query.Limit < 0 {
		return errors.New("codebase query limit is negative")
	}
	return nil
}

type Hit struct {
	Path      string
	StartLine int
	EndLine   int
	Snippet   string
	Score     float64
}

func (hit Hit) Validate() error {
	if strings.TrimSpace(hit.Path) == "" {
		return errors.New("codebase hit path is empty")
	}
	if hit.StartLine <= 0 || hit.EndLine < hit.StartLine {
		return fmt.Errorf("codebase hit %s has invalid line range %d..%d", hit.Path, hit.StartLine, hit.EndLine)
	}
	if math.IsNaN(hit.Score) || math.IsInf(hit.Score, 0) || hit.Score < 0 || hit.Score > 1 {
		return fmt.Errorf("codebase hit %s has score outside [0,1]", hit.Path)
	}
	return nil
}

type ReindexOperation struct{ ID string }

func (operation ReindexOperation) Validate() error {
	if strings.TrimSpace(operation.ID) == "" {
		return errors.New("codebase reindex operation id is empty")
	}
	return nil
}

type Service interface {
	Status(context.Context, string) (Status, error)
	Search(context.Context, Query) ([]Hit, error)
	Reindex(context.Context, string) (ReindexOperation, error)
}
