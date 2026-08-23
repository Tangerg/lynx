package agentmemory

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxFactsPerExtraction = 32
	MaxFactBytes          = 2 << 10
	MaxFactBatchBytes     = 32 << 10
	MaxLedgerFoldFacts    = 128
	MaxCurationItems      = 256
)

type ModelSelection struct {
	Provider string
	Model    string
}

func (selection ModelSelection) Validate() error {
	if strings.TrimSpace(selection.Provider) == "" ||
		strings.TrimSpace(selection.Provider) != selection.Provider ||
		strings.TrimSpace(selection.Model) == "" ||
		strings.TrimSpace(selection.Model) != selection.Model {
		return errors.New("agentmemory: model selection is incomplete")
	}
	return nil
}

// MaintenanceRun is one committed root Run eligible for fact extraction.
type MaintenanceRun struct {
	RunID      string
	SessionID  string
	Workspace  string
	Selection  ModelSelection
	FinishedAt time.Time
}

func (run MaintenanceRun) Validate() error {
	switch {
	case strings.TrimSpace(run.RunID) == "" || strings.TrimSpace(run.RunID) != run.RunID:
		return errors.New("agentmemory: maintenance run id is required")
	case strings.TrimSpace(run.SessionID) == "" ||
		strings.TrimSpace(run.SessionID) != run.SessionID:
		return errors.New("agentmemory: maintenance session id is required")
	case !filepath.IsAbs(run.Workspace) ||
		filepath.Clean(run.Workspace) != run.Workspace:
		return errors.New("agentmemory: maintenance workspace must be canonical and absolute")
	case run.FinishedAt.IsZero():
		return errors.New("agentmemory: maintenance finish time is required")
	}
	return run.Selection.Validate()
}

// FactBatch is the idempotent extraction result for one terminal root Run.
// An empty Facts slice is a durable completed extraction, not missing work.
type FactBatch struct {
	RunID      string
	SessionID  string
	Project    string
	Source     ModelSelection
	Extractor  *ModelSelection
	Day        string
	Facts      []string
	CapturedAt time.Time
}

func (batch FactBatch) Normalize() (FactBatch, error) {
	batch.RunID = strings.TrimSpace(batch.RunID)
	batch.SessionID = strings.TrimSpace(batch.SessionID)
	batch.Project = strings.TrimSpace(batch.Project)
	batch.CapturedAt = batch.CapturedAt.UTC()
	switch {
	case batch.RunID == "":
		return FactBatch{}, errors.New("agentmemory: fact batch run is required")
	case batch.SessionID == "":
		return FactBatch{}, errors.New("agentmemory: fact batch session is required")
	case !filepath.IsAbs(batch.Project) ||
		filepath.Clean(batch.Project) != batch.Project:
		return FactBatch{}, errors.New("agentmemory: fact batch project must be canonical and absolute")
	case batch.CapturedAt.IsZero():
		return FactBatch{}, errors.New("agentmemory: fact batch capture time is required")
	}
	if err := batch.Source.Validate(); err != nil {
		return FactBatch{}, err
	}
	day, err := time.Parse(time.DateOnly, batch.Day)
	if err != nil || day.Format(time.DateOnly) != batch.Day {
		return FactBatch{}, fmt.Errorf(
			"agentmemory: invalid ledger day %q",
			batch.Day,
		)
	}
	batch.Facts, err = NormalizeFacts(batch.Facts)
	if err != nil {
		return FactBatch{}, err
	}
	if batch.Extractor != nil {
		extractor := *batch.Extractor
		if err := extractor.Validate(); err != nil {
			return FactBatch{}, err
		}
		batch.Extractor = &extractor
	} else if len(batch.Facts) > 0 {
		return FactBatch{}, errors.New(
			"agentmemory: extracted facts require extractor provenance",
		)
	}
	return batch, nil
}

type LedgerFact struct {
	Sequence   int64
	RunID      string
	SessionID  string
	Day        string
	Content    string
	Selection  ModelSelection
	CapturedAt time.Time
}

func (fact LedgerFact) Validate() error {
	switch {
	case fact.Sequence <= 0:
		return errors.New("agentmemory: ledger sequence must be positive")
	case strings.TrimSpace(fact.RunID) == "" || strings.TrimSpace(fact.RunID) != fact.RunID:
		return errors.New("agentmemory: ledger run must be canonical")
	case strings.TrimSpace(fact.SessionID) == "" ||
		strings.TrimSpace(fact.SessionID) != fact.SessionID:
		return errors.New("agentmemory: ledger session must be canonical")
	case fact.CapturedAt.IsZero():
		return errors.New("agentmemory: ledger capture time is required")
	}
	if err := fact.Selection.Validate(); err != nil {
		return err
	}
	day, err := time.Parse(time.DateOnly, fact.Day)
	if err != nil || day.Format(time.DateOnly) != fact.Day {
		return fmt.Errorf("agentmemory: invalid ledger day %q", fact.Day)
	}
	values, err := NormalizeFacts([]string{fact.Content})
	if err != nil {
		return err
	}
	if len(values) != 1 || values[0] != fact.Content {
		return errors.New("agentmemory: ledger content is not canonical")
	}
	return nil
}

type CurationState struct {
	Watermark int64
	Revision  uint64
	UpdatedAt time.Time
}

func (state CurationState) Validate() error {
	switch {
	case state.Watermark < 0:
		return errors.New("agentmemory: curation watermark must not be negative")
	case state.Revision == 0 && (state.Watermark != 0 || !state.UpdatedAt.IsZero()):
		return errors.New("agentmemory: absent curation state has stored values")
	case state.Revision > 0 && state.UpdatedAt.IsZero():
		return errors.New("agentmemory: stored curation state requires update time")
	}
	return nil
}

func NewProposal(
	id string,
	project string,
	content string,
	sessionID string,
	now time.Time,
) (Item, error) {
	content = strings.TrimSpace(content)
	now = now.UTC()
	item := Item{
		ID: id, Scope: ScopeProject, Project: project, Content: content,
		Digest: Digest(content), Origin: OriginAuto, Status: StatusPending,
		SessionID: strings.TrimSpace(sessionID), Day: now.Format(time.DateOnly),
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func NormalizeFacts(input []string) ([]string, error) {
	if len(input) > MaxFactsPerExtraction {
		return nil, fmt.Errorf(
			"agentmemory: fact count exceeds %d",
			MaxFactsPerExtraction,
		)
	}
	values := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	total := 0
	for _, raw := range input {
		content := strings.TrimSpace(raw)
		if content == "" {
			continue
		}
		if len(content) > MaxFactBytes {
			return nil, fmt.Errorf(
				"agentmemory: fact exceeds %d bytes",
				MaxFactBytes,
			)
		}
		digest := Digest(content)
		if _, duplicate := seen[digest]; duplicate {
			continue
		}
		total += len(content)
		if total > MaxFactBatchBytes {
			return nil, fmt.Errorf(
				"agentmemory: fact batch exceeds %d bytes",
				MaxFactBatchBytes,
			)
		}
		seen[digest] = struct{}{}
		values = append(values, content)
	}
	return values, nil
}
