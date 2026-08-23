package memoryflow

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
)

const (
	maintenanceRunBatch     = 16
	maintenanceProjectBatch = 16
	minimumTranscriptBytes  = 80
	maximumTranscriptBytes  = 128 << 10
	maximumTranscriptBlock  = 8 << 10
	maximumFoldFactBytes    = 64 << 10
	minimumFactsBeforeFold  = 8
	maximumTimeBetweenFolds = 24 * time.Hour
)

type maintenanceStore interface {
	ListAgentMemoryMaintenanceRuns(
		context.Context,
		time.Time,
		int,
	) ([]agentmemory.MaintenanceRun, error)
	ListAgentMemoryRunMessages(
		context.Context,
		string,
	) ([]conversationdomain.Record, error)
	CommitAgentMemoryExtraction(
		context.Context,
		agentmemory.FactBatch,
	) error
	MarkAgentMemoryExtractionAttempt(context.Context, string, time.Time) error
	ListAgentMemoryCurationProjects(
		context.Context,
		int,
		time.Time,
		int,
	) ([]string, error)
	GetAgentMemoryCurationState(
		context.Context,
		string,
	) (agentmemory.CurationState, error)
	ListAgentMemoryLedger(
		context.Context,
		string,
		int64,
		int,
	) ([]agentmemory.LedgerFact, error)
	ListAgentMemoryCurationItems(
		context.Context,
		string,
	) ([]agentmemory.Item, error)
	CommitAgentMemoryCuration(
		context.Context,
		string,
		int64,
		int64,
		[]agentmemory.Item,
		time.Time,
	) (published bool, won bool, err error)
}

type MaintenanceModels interface {
	ExtractMemoryFacts(
		context.Context,
		agentmemory.ModelSelection,
		string,
	) (agentmemory.ModelSelection, []string, error)
	CurateMemoryFacts(
		context.Context,
		agentmemory.ModelSelection,
		[]agentmemory.Item,
		[]agentmemory.LedgerFact,
	) ([]string, error)
}

// RunSettled is a non-blocking lifecycle signal. Durable queries decide which
// committed root Runs still need extraction, so the signal carries no identity.
func (service *Service) RunSettled() {
	service.wakeMaintenance()
}

// Recover schedules the same durable backlog scan used after a live Run. A
// crash between terminal Run commit and this signal therefore loses no work.
func (service *Service) Recover() {
	service.wakeMaintenance()
}

func (service *Service) wakeMaintenance() {
	select {
	case <-service.lifetime.Done():
		return
	default:
	}
	select {
	case service.wake <- struct{}{}:
	default:
	}
}

func (service *Service) maintenanceLoop() {
	defer service.tasks.Done()
	for {
		select {
		case <-service.lifetime.Done():
			return
		case <-service.wake:
			service.maintain(service.lifetime)
		}
	}
}

func (service *Service) maintain(ctx context.Context) {
	if service.extractionSweep.IsZero() {
		service.extractionSweep = service.now().UTC()
	}
	runs, err := service.store.ListAgentMemoryMaintenanceRuns(
		ctx,
		service.extractionSweep,
		maintenanceRunBatch,
	)
	if err != nil {
		service.logMaintenanceError(ctx, "list_runs", "", err)
		return
	}
	settledExtractionAttempts := 0
	for _, run := range runs {
		err := service.extractRun(ctx, run)
		if err != nil {
			service.logMaintenanceError(ctx, "extract", run.RunID, err)
			attemptedAt := service.now().UTC()
			if attemptedAt.Before(service.extractionSweep) {
				attemptedAt = service.extractionSweep
			}
			if markErr := service.store.MarkAgentMemoryExtractionAttempt(
				ctx,
				run.RunID,
				attemptedAt,
			); markErr != nil {
				service.logMaintenanceError(ctx, "mark_attempt", run.RunID, markErr)
			} else {
				settledExtractionAttempts++
			}
		} else {
			settledExtractionAttempts++
		}
		if ctx.Err() != nil {
			return
		}
	}

	now := service.now().UTC()
	projects, err := service.store.ListAgentMemoryCurationProjects(
		ctx,
		minimumFactsBeforeFold,
		now.Add(-maximumTimeBetweenFolds),
		maintenanceProjectBatch,
	)
	if err != nil {
		service.logMaintenanceError(ctx, "list_projects", "", err)
		return
	}
	committedCurations := 0
	for _, project := range projects {
		committed, err := service.curateProject(ctx, project)
		if err != nil {
			service.logMaintenanceError(ctx, "curate", project, err)
		} else if committed {
			committedCurations++
		}
		if ctx.Err() != nil {
			return
		}
	}
	if len(runs) == maintenanceRunBatch && settledExtractionAttempts > 0 {
		service.wakeMaintenance()
	} else if len(runs) < maintenanceRunBatch {
		service.extractionSweep = time.Time{}
	}
	if len(projects) == maintenanceProjectBatch && committedCurations > 0 {
		service.wakeMaintenance()
	}
}

func (service *Service) extractRun(
	ctx context.Context,
	run agentmemory.MaintenanceRun,
) error {
	if err := run.Validate(); err != nil {
		return err
	}
	project, err := service.project(ctx, run.Workspace)
	if err != nil {
		return err
	}
	records, err := service.store.ListAgentMemoryRunMessages(ctx, run.RunID)
	if err != nil {
		return err
	}
	transcript, useful, err := renderMaintenanceTranscript(run, records)
	if err != nil {
		return err
	}
	facts := []string(nil)
	var extractor *agentmemory.ModelSelection
	if useful {
		selection, extracted, err := service.maintenance.ExtractMemoryFacts(
			ctx,
			run.Selection,
			transcript,
		)
		if err != nil {
			return err
		}
		extractor = &selection
		facts = extracted
	}
	batch, err := (agentmemory.FactBatch{
		RunID: run.RunID, SessionID: run.SessionID, Project: project,
		Source: run.Selection, Extractor: extractor,
		Day:   run.FinishedAt.UTC().Format(time.DateOnly),
		Facts: facts, CapturedAt: service.now().UTC(),
	}).Normalize()
	if err != nil {
		return err
	}
	return service.store.CommitAgentMemoryExtraction(ctx, batch)
}

func (service *Service) curateProject(
	ctx context.Context,
	project string,
) (bool, error) {
	state, err := service.store.GetAgentMemoryCurationState(ctx, project)
	if err != nil {
		return false, err
	}
	if err := state.Validate(); err != nil {
		return false, err
	}
	facts, err := service.store.ListAgentMemoryLedger(
		ctx,
		project,
		state.Watermark,
		agentmemory.MaxLedgerFoldFacts,
	)
	if err != nil || len(facts) == 0 {
		return false, err
	}
	now := service.now().UTC()
	if !curationDue(state, len(facts), now) {
		return false, nil
	}
	facts = boundLedgerFacts(facts, maximumFoldFactBytes)
	if len(facts) == 0 {
		return false, errors.New("memoryflow: curation ledger exceeded its bounded window")
	}
	current, err := service.store.ListAgentMemoryCurationItems(ctx, project)
	if err != nil {
		return false, err
	}
	selection := facts[len(facts)-1].Selection
	contents, err := service.maintenance.CurateMemoryFacts(
		ctx,
		selection,
		current,
		facts,
	)
	if err != nil {
		return false, err
	}
	contents, err = agentmemory.NormalizeFacts(contents)
	if err != nil {
		return false, err
	}
	proposals := make([]agentmemory.Item, len(contents))
	sourceSession := facts[len(facts)-1].SessionID
	for index, content := range contents {
		id, err := service.ids.New("mem_")
		if err != nil {
			return false, err
		}
		proposals[index], err = agentmemory.NewProposal(
			id,
			project,
			content,
			sourceSession,
			now,
		)
		if err != nil {
			return false, err
		}
	}
	published, committed, err := service.store.CommitAgentMemoryCuration(
		ctx,
		project,
		state.Watermark,
		facts[len(facts)-1].Sequence,
		proposals,
		now,
	)
	if err != nil {
		return false, err
	}
	if published {
		service.publish()
	}
	return committed, nil
}

func boundLedgerFacts(
	facts []agentmemory.LedgerFact,
	limit int,
) []agentmemory.LedgerFact {
	selected := make([]agentmemory.LedgerFact, 0, len(facts))
	used := 0
	for _, fact := range facts {
		required := len(fact.Content)
		if len(selected) > 0 {
			required++
		}
		if required > limit-used {
			break
		}
		selected = append(selected, fact)
		used += required
	}
	return selected
}

func curationDue(
	state agentmemory.CurationState,
	pending int,
	now time.Time,
) bool {
	if pending == 0 {
		return false
	}
	if state.Revision == 0 || pending >= minimumFactsBeforeFold {
		return true
	}
	return !state.UpdatedAt.IsZero() &&
		now.Sub(state.UpdatedAt) >= maximumTimeBetweenFolds
}

func (service *Service) logMaintenanceError(
	ctx context.Context,
	stage string,
	identity string,
	err error,
) {
	if err == nil || ctx.Err() != nil {
		return
	}
	service.logger.WarnContext(
		ctx,
		"AgentMemory maintenance did not complete",
		"stage",
		stage,
		"identity",
		identity,
		"error",
		err,
	)
}

func (service *Service) Close() {
	if service == nil {
		return
	}
	service.closeOnce.Do(service.cancel)
	service.tasks.Wait()
}
