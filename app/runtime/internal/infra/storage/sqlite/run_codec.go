package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// The Run row's two JSON columns. Token accounting and a failure classification
// are read and written whole with the row, never queried across, so they stay one
// value each rather than a dozen columns — the same call the goals table makes
// for its budget. They are declared here, with explicit names, because the
// durable encoding is this adapter's to choose: the domain values carry no tags,
// and renaming a Go field must not silently invalidate stored rows.
type runUsageRow struct {
	InputTokens      int64                     `json:"inputTokens,omitzero"`
	OutputTokens     int64                     `json:"outputTokens,omitzero"`
	CacheReadTokens  int64                     `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64                     `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64                     `json:"reasoningTokens,omitzero"`
	CostUSD          *float64                  `json:"costUsd,omitempty"`
	ByModel          map[string]runModelRowUse `json:"byModel,omitempty"`
}

type runModelRowUse struct {
	InputTokens      int64    `json:"inputTokens,omitzero"`
	OutputTokens     int64    `json:"outputTokens,omitzero"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitzero"`
	CostUSD          *float64 `json:"costUsd,omitempty"`
}

// runAccountingRow is the parked Run's consumption and allowance, encoded as the
// one value a continuation needs to pick the Run back up where it left off. It
// reuses runUsageRow rather than spelling usage a second way, so the two carriers
// of a Run's accounting agree by construction.
type runAccountingRow struct {
	Steps            int          `json:"steps,omitzero"`
	ActiveDurationNs int64        `json:"activeDurationNs,omitzero"`
	Usage            *runUsageRow `json:"usage,omitempty"`
	MaxSteps         int          `json:"maxSteps,omitzero"`
	MaxBudgetUSD     float64      `json:"maxBudgetUsd,omitzero"`
}

func encodeRunAccounting(metrics transcript.RunMetrics, limits execution.RunLimits) (string, error) {
	encoded, err := json.Marshal(runAccountingRow{
		Steps:            metrics.Steps,
		ActiveDurationNs: int64(metrics.ActiveDuration),
		Usage:            runUsageRowOf(metrics.Usage),
		MaxSteps:         limits.MaxSteps,
		MaxBudgetUSD:     limits.MaxBudgetUSD,
	})
	if err != nil {
		return "", fmt.Errorf("encode run accounting: %w", err)
	}
	return string(encoded), nil
}

func decodeRunAccounting(encoded string) (transcript.RunMetrics, execution.RunLimits, error) {
	if encoded == "" {
		return transcript.RunMetrics{}, execution.RunLimits{}, nil
	}
	var row runAccountingRow
	if err := json.Unmarshal([]byte(encoded), &row); err != nil {
		return transcript.RunMetrics{}, execution.RunLimits{}, fmt.Errorf("decode run accounting: %w", err)
	}
	metrics := transcript.RunMetrics{
		Usage:          row.Usage.usage(),
		Steps:          row.Steps,
		ActiveDuration: time.Duration(row.ActiveDurationNs),
	}
	return metrics, execution.RunLimits{MaxSteps: row.MaxSteps, MaxBudgetUSD: row.MaxBudgetUSD}, nil
}

type runProblemRow struct {
	Kind              int    `json:"kind"`
	Detail            string `json:"detail,omitempty"`
	DocURL            string `json:"docUrl,omitempty"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitzero"`
}

// metricsValues are one Run's accumulated consumption, encoded and ready to bind
// to a statement. An empty usage string means the Run has recorded none yet.
type metricsValues struct {
	steps      int
	durationNs int64
	usage      string
}

func runMetricsRow(metrics transcript.RunMetrics) (metricsValues, error) {
	usage, err := encodeRunUsage(metrics.Usage)
	if err != nil {
		return metricsValues{}, err
	}
	return metricsValues{
		steps:      metrics.Steps,
		durationNs: int64(metrics.ActiveDuration),
		usage:      usage,
	}, nil
}

func encodeRunUsage(usage *transcript.Usage) (string, error) {
	row := runUsageRowOf(usage)
	if row == nil {
		return "", nil
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("encode run usage: %w", err)
	}
	return string(encoded), nil
}

func runUsageRowOf(usage *transcript.Usage) *runUsageRow {
	if usage == nil {
		return nil
	}
	row := &runUsageRow{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens:  usage.ReasoningTokens,
		CostUSD:          usage.CostUSD,
	}
	if len(usage.ByModel) > 0 {
		row.ByModel = make(map[string]runModelRowUse, len(usage.ByModel))
		for model, perModel := range usage.ByModel {
			row.ByModel[model] = runModelRowUse{
				InputTokens:      perModel.InputTokens,
				OutputTokens:     perModel.OutputTokens,
				CacheReadTokens:  perModel.CacheReadTokens,
				CacheWriteTokens: perModel.CacheWriteTokens,
				ReasoningTokens:  perModel.ReasoningTokens,
				CostUSD:          perModel.CostUSD,
			}
		}
	}
	return row
}

func encodeRunProblem(problem *transcript.Problem) (string, error) {
	if problem == nil {
		return "", nil
	}
	encoded, err := json.Marshal(runProblemRow{
		Kind:              int(problem.Kind),
		Detail:            problem.Detail,
		DocURL:            problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds,
	})
	if err != nil {
		return "", fmt.Errorf("encode run problem: %w", err)
	}
	return string(encoded), nil
}

// scanRun decodes one Run row plus the joined open-interrupt payload.
//
// The fine [execution.RunState] is rebuilt from the coarse admission state and
// the terminal reason beside it rather than stored a second time, and the
// terminal facts are materialized exactly when the state says they exist — the
// equivalence [transcript.Run.Validate] enforces on the way in.
func scanRun(row scanRow) (transcript.Run, error) {
	var (
		run                 transcript.Run
		coarse              string
		outcome             string
		provider            string
		model               string
		usage               string
		problem             string
		durationNs          int64
		startedAt           int64
		finishedAt          int64
		updatedAt           int64
		interruptsSuspended sql.NullString
	)
	if err := row.Scan(
		&run.ID, &run.SessionID, &run.SpawnedByItemID, &coarse, &run.ActiveSegmentID, &outcome,
		&provider, &model, &run.Detail, &run.Metrics.Steps, &durationNs, &usage, &problem,
		&run.Limits.MaxSteps, &run.Limits.MaxBudgetUSD,
		&run.MessageMark, &startedAt, &finishedAt, &updatedAt, &interruptsSuspended,
	); err != nil {
		return transcript.Run{}, fmt.Errorf("sqlite: scan run: %w", err)
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return transcript.Run{}, fmt.Errorf("sqlite: decode run %q model selection: %w", run.ID, err)
	}
	run.ModelSelection = selection
	run.CreatedAt = time.Unix(0, startedAt).UTC()
	run.UpdatedAt = time.Unix(0, updatedAt).UTC()
	// Consumption is read for every state, not only the terminal one: a running
	// Run has already spent tokens and a parked one committed what it spent.
	run.Metrics.ActiveDuration = time.Duration(durationNs)
	if run.Metrics.Usage, err = decodeRunUsage(usage); err != nil {
		return transcript.Run{}, fmt.Errorf("sqlite: decode run %q usage: %w", run.ID, err)
	}

	switch coarse {
	case runStateRunning:
		run.State = execution.Running
	case runStateInterrupted:
		run.State = execution.Interrupted
		// A parked Run's interrupts are what it is parked ON. The absence of the
		// interrupt record it was parked with is a broken park, not a Run waiting on
		// nothing — reporting it as an empty wait would invent a state the run never
		// had. Boot reconciliation is what resolves it.
		if !interruptsSuspended.Valid {
			return transcript.Run{}, fmt.Errorf("sqlite: run %q is parked with no open interrupt", run.ID)
		}
		if run.Interrupts, err = decodeInterrupts(interruptsSuspended.String); err != nil {
			return transcript.Run{}, fmt.Errorf("sqlite: decode run %q interrupts: %w", run.ID, err)
		}
		if len(run.Interrupts) == 0 {
			return transcript.Run{}, fmt.Errorf("sqlite: run %q is parked on an empty interrupt set", run.ID)
		}
	case runStateTerminal:
		reason, ok := execution.ParseOutcome(outcome)
		if !ok {
			return transcript.Run{}, fmt.Errorf("sqlite: run %q has unknown outcome %q", run.ID, outcome)
		}
		state, ok := execution.Running.Terminate(reason)
		if !ok {
			return transcript.Run{}, fmt.Errorf("sqlite: run %q outcome %s reaches no terminal state", run.ID, reason)
		}
		run.State = state
		run.Outcome = &reason
		run.FinishedAt = time.Unix(0, finishedAt).UTC()
		if run.Error, err = decodeRunProblem(problem); err != nil {
			return transcript.Run{}, fmt.Errorf("sqlite: decode run %q problem: %w", run.ID, err)
		}
	default:
		return transcript.Run{}, fmt.Errorf("sqlite: run %q has unknown state %q", run.ID, coarse)
	}
	if err := run.Validate(); err != nil {
		return transcript.Run{}, fmt.Errorf("sqlite: run %q: %w", run.ID, err)
	}
	return run, nil
}

func decodeRunUsage(encoded string) (*transcript.Usage, error) {
	if encoded == "" {
		return nil, nil
	}
	var row runUsageRow
	if err := json.Unmarshal([]byte(encoded), &row); err != nil {
		return nil, err
	}
	return row.usage(), nil
}

func (row *runUsageRow) usage() *transcript.Usage {
	if row == nil {
		return nil
	}
	usage := &transcript.Usage{ModelUsage: transcript.ModelUsage{
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		CacheReadTokens:  row.CacheReadTokens,
		CacheWriteTokens: row.CacheWriteTokens,
		ReasoningTokens:  row.ReasoningTokens,
		CostUSD:          row.CostUSD,
	}}
	if len(row.ByModel) > 0 {
		usage.ByModel = make(map[string]transcript.ModelUsage, len(row.ByModel))
		for model, perModel := range row.ByModel {
			usage.ByModel[model] = transcript.ModelUsage{
				InputTokens:      perModel.InputTokens,
				OutputTokens:     perModel.OutputTokens,
				CacheReadTokens:  perModel.CacheReadTokens,
				CacheWriteTokens: perModel.CacheWriteTokens,
				ReasoningTokens:  perModel.ReasoningTokens,
				CostUSD:          perModel.CostUSD,
			}
		}
	}
	return usage
}

func decodeRunProblem(encoded string) (*transcript.Problem, error) {
	if encoded == "" {
		return nil, nil
	}
	var row runProblemRow
	if err := json.Unmarshal([]byte(encoded), &row); err != nil {
		return nil, err
	}
	// Scope is not stored: a problem in a Run's result slot is a Run problem by
	// definition, and Validate refuses any other.
	return &transcript.Problem{
		Kind:              transcript.ProblemKind(row.Kind),
		Scope:             transcript.RunProblem,
		Detail:            row.Detail,
		DocURL:            row.DocURL,
		RetryAfterSeconds: row.RetryAfterSeconds,
	}, nil
}
