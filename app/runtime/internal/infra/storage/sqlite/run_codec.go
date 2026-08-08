package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// The Run row's JSON columns. Token accounting, a failure classification, and
// frozen capabilities are read and written whole with the row, never queried
// across, so they remain focused values instead of expanding into incidental
// columns. Their explicit adapter rows keep Go field names from defining the
// durable format.
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
	MaxTotalTokens   int64        `json:"maxTotalTokens,omitzero"`
	MaxSteps         int          `json:"maxSteps,omitzero"`
	MaxBudgetUSD     float64      `json:"maxBudgetUsd,omitzero"`
}

func runAccountingRowOf(metrics transcript.RunMetrics, limits rundomain.RunLimits) runAccountingRow {
	return runAccountingRow{
		Steps:            metrics.Steps,
		ActiveDurationNs: int64(metrics.ActiveDuration),
		Usage:            runUsageRowOf(metrics.Usage),
		MaxTotalTokens:   limits.MaxTotalTokens,
		MaxSteps:         limits.MaxSteps,
		MaxBudgetUSD:     limits.MaxBudgetUSD,
	}
}

func (row runAccountingRow) values() (transcript.RunMetrics, rundomain.RunLimits, error) {
	metrics := transcript.RunMetrics{
		Usage:          row.Usage.usage(),
		Steps:          row.Steps,
		ActiveDuration: time.Duration(row.ActiveDurationNs),
	}
	limits := rundomain.RunLimits{
		MaxTotalTokens: row.MaxTotalTokens, MaxSteps: row.MaxSteps, MaxBudgetUSD: row.MaxBudgetUSD,
	}
	if err := metrics.Validate(); err != nil {
		return transcript.RunMetrics{}, rundomain.RunLimits{}, fmt.Errorf("metrics: %w", err)
	}
	if err := limits.Validate(); err != nil {
		return transcript.RunMetrics{}, rundomain.RunLimits{}, fmt.Errorf("limits: %w", err)
	}
	return metrics, limits, nil
}

// runCapabilitiesRow is the Run's frozen optional behavior. Interrupt kinds are
// stored under their canonical names rather than ordinals, so inserting a kind
// into the enum cannot silently re-label stored rows.
type runCapabilitiesRow struct {
	ChildRuns      bool     `json:"childRuns,omitempty"`
	InterruptKinds []string `json:"interruptKinds,omitempty"`
}

// encodeRunCapabilities returns the empty string for no optional capabilities,
// keeping one representation instead of both null and an empty object.
func encodeRunCapabilities(capabilities rundomain.RunCapabilities) (string, error) {
	if err := capabilities.Validate(); err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	if capabilities.IsEmpty() {
		return "", nil
	}
	row := runCapabilitiesRow{ChildRuns: capabilities.ChildRuns}
	for _, kind := range capabilities.InterruptKinds {
		row.InterruptKinds = append(row.InterruptKinds, kind.String())
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	return string(encoded), nil
}

func decodeRunCapabilities(encoded string) (rundomain.RunCapabilities, error) {
	if encoded == "" {
		return rundomain.RunCapabilities{}, nil
	}
	var row runCapabilitiesRow
	decoder := json.NewDecoder(bytes.NewReader([]byte(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return rundomain.RunCapabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return rundomain.RunCapabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	capabilities := rundomain.RunCapabilities{ChildRuns: row.ChildRuns}
	for _, name := range row.InterruptKinds {
		kind, ok := interrupt.ParseKind(name)
		if !ok {
			// A stored kind this build cannot raise would let the Run park on
			// something nothing answers. Refusing the row is the honest outcome.
			return rundomain.RunCapabilities{}, fmt.Errorf("decode run capabilities: unknown interrupt kind %q", name)
		}
		capabilities.InterruptKinds = append(capabilities.InterruptKinds, kind)
	}
	if err := capabilities.Validate(); err != nil {
		return rundomain.RunCapabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	return capabilities, nil
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

// pendingReadPolicy says whether an Waiting Run must have its root-owned
// Pending set. Ordinary reads require it because a complete parked
// Run is inseparable from that set. Boot recovery is the one exception: it must
// still be able to read and terminalize a row whose pending set was lost in the
// crash it is repairing.
type pendingReadPolicy uint8

const (
	requirePendingSet pendingReadPolicy = iota
	allowMissingPendingSet
)

// scanRun decodes one complete Run row plus the joined open-interrupt payload.
//
// The fine [rundomain.RunState] is rebuilt from the coarse admission state and
// the terminal reason beside it rather than stored a second time, and the
// terminal facts are materialized exactly when the state says they exist — the
// equivalence [transcript.Run.Validate] enforces on the way in.
func scanRun(row scanRow) (transcript.Run, error) {
	return scanRunRow(row, requirePendingSet)
}

// scanRunForRecovery uses the same complete durable Run decoder as every normal
// read, but tolerates the one broken relation reconciliation exists to repair:
// an Waiting row whose root-owned pending set is missing.
func scanRunForRecovery(row scanRow) (transcript.Run, error) {
	return scanRunRow(row, allowMissingPendingSet)
}

func scanRunRow(row scanRow, pendingPolicy pendingReadPolicy) (transcript.Run, error) {
	var (
		run                 transcript.Run
		coarse              string
		outcome             string
		provider            string
		model               string
		usage               string
		problem             string
		ownCapabilities     string
		rootCapabilities    sql.NullString
		durationNs          int64
		startedAt           int64
		finishedAt          int64
		updatedAt           int64
		interruptsSuspended sql.NullString
	)
	if err := row.Scan(
		&run.ID, &run.SessionID,
		&run.SpawnedByItemID, &run.ParentRunID, &run.RootRunID,
		&coarse, &run.ActiveSegmentID, &outcome,
		&provider, &model, &run.GoalLeaseID, &run.Detail, &run.Metrics.Steps, &durationNs, &usage, &problem,
		&run.Limits.MaxTotalTokens, &run.Limits.MaxSteps, &run.Limits.MaxBudgetUSD, &ownCapabilities, &rootCapabilities,
		&run.MessageMark, &startedAt, &finishedAt, &updatedAt, &interruptsSuspended,
	); err != nil {
		return transcript.Run{}, fmt.Errorf("scan run row: %w", err)
	}
	capabilities := ownCapabilities
	if run.Lineage().IsChild() {
		if ownCapabilities != "" {
			return transcript.Run{}, fmt.Errorf("child run %q stores capabilities of its own", run.ID)
		}
		if !rootCapabilities.Valid {
			return transcript.Run{}, fmt.Errorf(
				"child run %q references missing root %q",
				run.ID,
				run.RootRunID,
			)
		}
		capabilities = rootCapabilities.String
	}
	capabilitiesValue, err := decodeRunCapabilities(capabilities)
	if err != nil {
		return transcript.Run{}, fmt.Errorf("run %q: %w", run.ID, err)
	}
	run.Capabilities = capabilitiesValue
	selection, err := modelref.New(provider, model)
	if err != nil {
		return transcript.Run{}, fmt.Errorf("decode run %q model selection: %w", run.ID, err)
	}
	run.ModelSelection = selection
	run.CreatedAt = time.Unix(0, startedAt).UTC()
	run.UpdatedAt = time.Unix(0, updatedAt).UTC()
	// Consumption is read for every state, not only the terminal one: a running
	// Run has already spent tokens and a parked one committed what it spent.
	run.Metrics.ActiveDuration = time.Duration(durationNs)
	if run.Metrics.Usage, err = decodeRunUsage(usage); err != nil {
		return transcript.Run{}, fmt.Errorf("decode run %q usage: %w", run.ID, err)
	}

	switch coarse {
	case runStateRunning:
		run.State = rundomain.Running
	case runStateWaiting:
		run.State = rundomain.Waiting
		// Every suspended Run must join its root-owned pending set. Only interrupts
		// raised by this Run are projected onto it; an empty filtered result means
		// the Run was suspended by another source in the tree.
		if !interruptsSuspended.Valid {
			if pendingPolicy == requirePendingSet {
				return transcript.Run{}, fmt.Errorf("run %q is waiting with no root-owned Pending set", run.ID)
			}
			break
		}
		treeInterrupts, decodeErr := decodeInterrupts(interruptsSuspended.String)
		if decodeErr != nil {
			err = decodeErr
			return transcript.Run{}, fmt.Errorf("decode run %q interrupts: %w", run.ID, err)
		}
		for _, interrupt := range treeInterrupts {
			if interrupt.RunID == run.ID {
				run.Interrupts = append(run.Interrupts, interrupt)
			}
		}
	case runStateTerminal:
		reason, ok := rundomain.ParseOutcome(outcome)
		if !ok {
			return transcript.Run{}, fmt.Errorf("run %q has unknown outcome %q", run.ID, outcome)
		}
		state, ok := rundomain.Running.Terminate(reason)
		if !ok {
			return transcript.Run{}, fmt.Errorf("run %q outcome %s reaches no terminal state", run.ID, reason)
		}
		run.State = state
		run.Outcome = &reason
		run.FinishedAt = time.Unix(0, finishedAt).UTC()
		if run.Error, err = decodeRunProblem(problem); err != nil {
			return transcript.Run{}, fmt.Errorf("decode run %q problem: %w", run.ID, err)
		}
	default:
		return transcript.Run{}, fmt.Errorf("run %q has unknown state %q", run.ID, coarse)
	}
	if err := run.Validate(); err != nil {
		return transcript.Run{}, fmt.Errorf("run %q: %w", run.ID, err)
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
	kind, err := decodeRunProblemKind(row.Kind)
	if err != nil {
		return nil, err
	}
	// Scope is not stored: a problem in a Run's result slot is a Run problem by
	// definition, and Validate refuses any other.
	return &transcript.Problem{
		Kind:              kind,
		Scope:             transcript.RunProblem,
		Detail:            row.Detail,
		DocURL:            row.DocURL,
		RetryAfterSeconds: row.RetryAfterSeconds,
	}, nil
}

func decodeRunProblemKind(kind int) (transcript.ProblemKind, error) {
	switch kind {
	case int(transcript.InternalProblem):
		return transcript.InternalProblem, nil
	case int(transcript.RunLostProblem):
		return transcript.RunLostProblem, nil
	case int(transcript.AgentStuckProblem):
		return transcript.AgentStuckProblem, nil
	case int(transcript.RateLimitedProblem):
		return transcript.RateLimitedProblem, nil
	case int(transcript.InvalidAPIKeyProblem):
		return transcript.InvalidAPIKeyProblem, nil
	case int(transcript.TimeoutProblem):
		return transcript.TimeoutProblem, nil
	case int(transcript.ProviderUnavailableProblem):
		return transcript.ProviderUnavailableProblem, nil
	case int(transcript.ProviderRejectedProblem):
		return transcript.ProviderRejectedProblem, nil
	case int(transcript.DeniedByUserProblem):
		return transcript.DeniedByUserProblem, nil
	case int(transcript.ToolFailedProblem):
		return transcript.ToolFailedProblem, nil
	case int(transcript.ChildRunCanceledProblem):
		return transcript.ChildRunCanceledProblem, nil
	default:
		return 0, fmt.Errorf("unknown run problem kind %d", kind)
	}
}
