package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
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

func runAccountingRowOf(metrics rundomain.Metrics, limits rundomain.Limits) runAccountingRow {
	usage, reported := metrics.Usage()
	var usageRef *accounting.Usage
	if reported {
		usageRef = &usage
	}
	return runAccountingRow{
		Steps:            metrics.Steps(),
		ActiveDurationNs: int64(metrics.ActiveDuration()),
		Usage:            runUsageRowOf(usageRef),
		MaxTotalTokens:   limits.MaxTotalTokens,
		MaxSteps:         limits.MaxSteps,
		MaxBudgetUSD:     limits.MaxBudgetUSD,
	}
}

func (r runAccountingRow) values() (rundomain.Metrics, rundomain.Limits, error) {
	metrics, err := rundomain.NewMetrics(r.Usage.usage(), r.Steps, time.Duration(r.ActiveDurationNs))
	if err != nil {
		return rundomain.Metrics{}, rundomain.Limits{}, fmt.Errorf("metrics: %w", err)
	}
	limits := rundomain.Limits{
		MaxTotalTokens: r.MaxTotalTokens, MaxSteps: r.MaxSteps, MaxBudgetUSD: r.MaxBudgetUSD,
	}
	if err := limits.Validate(); err != nil {
		return rundomain.Metrics{}, rundomain.Limits{}, fmt.Errorf("limits: %w", err)
	}
	return metrics, limits, nil
}

// runCapabilitiesRow is the Run's frozen optional behavior. Interrupt kinds are
// stored under their canonical names rather than ordinals, so inserting a kind
// into the enum cannot silently re-label stored rows.
type runCapabilitiesRow struct {
	ChildRuns      bool             `json:"childRuns,omitempty"`
	InterruptKinds []interrupt.Kind `json:"interruptKinds,omitempty"`
}

// encodeRunCapabilities returns the empty string for no optional capabilities,
// keeping one representation instead of both null and an empty object.
func encodeRunCapabilities(capabilities rundomain.Capabilities) (string, error) {
	if err := capabilities.Validate(); err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	if capabilities.IsEmpty() {
		return "", nil
	}
	row := runCapabilitiesRow{ChildRuns: capabilities.ChildRuns}
	row.InterruptKinds = append(row.InterruptKinds, capabilities.InterruptKinds...)
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("encode run capabilities: %w", err)
	}
	return string(encoded), nil
}

func decodeRunCapabilities(encoded string) (rundomain.Capabilities, error) {
	if encoded == "" {
		return rundomain.Capabilities{}, nil
	}
	var row runCapabilitiesRow
	decoder := json.NewDecoder(bytes.NewReader([]byte(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	capabilities := rundomain.Capabilities{ChildRuns: row.ChildRuns}
	for _, kind := range row.InterruptKinds {
		if !kind.Valid() {
			// A stored kind this build cannot raise would let the Run park on
			// something nothing answers. Refusing the row is the honest outcome.
			return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: unknown interrupt kind %q", kind)
		}
		capabilities.InterruptKinds = append(capabilities.InterruptKinds, kind)
	}
	if err := capabilities.Validate(); err != nil {
		return rundomain.Capabilities{}, fmt.Errorf("decode run capabilities: %w", err)
	}
	return capabilities, nil
}

type runProblemRow struct {
	Kind              rundomain.FailureKind `json:"kind"`
	Detail            string                `json:"detail,omitempty"`
	DocURL            string                `json:"docUrl,omitempty"`
	RetryAfterSeconds int                   `json:"retryAfterSeconds,omitzero"`
}

// metricsValues are one Run's accumulated consumption, encoded and ready to bind
// to a statement. An empty usage string means the Run has recorded none yet.
type metricsValues struct {
	steps      int
	durationNs int64
	usage      string
}

func runMetricsRow(metrics rundomain.Metrics) (metricsValues, error) {
	reported, ok := metrics.Usage()
	var usage *accounting.Usage
	if ok {
		usage = &reported
	}
	encoded, err := encodeRunUsage(usage)
	if err != nil {
		return metricsValues{}, err
	}
	return metricsValues{
		steps:      metrics.Steps(),
		durationNs: int64(metrics.ActiveDuration()),
		usage:      encoded,
	}, nil
}

func encodeRunUsage(usage *accounting.Usage) (string, error) {
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

func runUsageRowOf(usage *accounting.Usage) *runUsageRow {
	if usage == nil {
		return nil
	}
	row := &runUsageRow{
		InputTokens:      usage.Total.InputTokens,
		OutputTokens:     usage.Total.OutputTokens,
		CacheReadTokens:  usage.Total.CacheReadTokens,
		CacheWriteTokens: usage.Total.CacheWriteTokens,
		ReasoningTokens:  usage.Total.ReasoningTokens,
		CostUSD:          usage.Total.CostUSD,
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

func encodeRunFailure(failure *rundomain.Failure) (string, error) {
	if failure == nil {
		return "", nil
	}
	if err := failure.Validate(); err != nil {
		return "", fmt.Errorf("encode run failure: %w", err)
	}
	encoded, err := json.Marshal(runProblemRow{
		Kind:              failure.Kind,
		Detail:            failure.Detail,
		DocURL:            failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter / time.Second),
	})
	if err != nil {
		return "", fmt.Errorf("encode run failure: %w", err)
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
// The fine [rundomain.State] is rebuilt from the coarse admission state and
// the terminal reason beside it rather than stored a second time, and the
// terminal facts are materialized exactly when the state says they exist — the
// equivalence [rundomain.Run.Validate] enforces on the way in.
func scanRun(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, requirePendingSet)
}

// scanRunForRecovery uses the same complete durable Run decoder as every normal
// read, but tolerates the one broken relation reconciliation exists to repair:
// an Waiting row whose root-owned pending set is missing.
func scanRunForRecovery(row scanRow) (rundomain.Run, error) {
	return scanRunRow(row, allowMissingPendingSet)
}

func scanRunRow(row scanRow, pendingPolicy pendingReadPolicy) (rundomain.Run, error) {
	var (
		id                  string
		sessionID           string
		spawnedByItemID     string
		parentRunID         string
		rootRunID           string
		coarse              string
		activeSegmentID     string
		outcome             string
		provider            string
		model               string
		goalIncarnationID   string
		detail              string
		steps               int
		usage               string
		contextTokens       int64
		problem             string
		limits              rundomain.Limits
		messageMark         int
		ownCapabilities     string
		rootCapabilities    sql.NullString
		durationNs          int64
		startedAt           int64
		finishedAt          int64
		updatedAt           int64
		interruptsSuspended sql.NullString
	)
	if err := row.Scan(
		&id, &sessionID,
		&spawnedByItemID, &parentRunID, &rootRunID,
		&coarse, &activeSegmentID, &outcome,
		&provider, &model, &goalIncarnationID, &detail,
		&steps, &durationNs, &usage, &contextTokens, &problem,
		&limits.MaxTotalTokens, &limits.MaxSteps, &limits.MaxBudgetUSD, &ownCapabilities, &rootCapabilities,
		&messageMark, &startedAt, &finishedAt, &updatedAt, &interruptsSuspended,
	); err != nil {
		return rundomain.Run{}, fmt.Errorf("scan run row: %w", err)
	}
	lineage := rundomain.Lineage{SpawnedByItemID: spawnedByItemID, ParentRunID: parentRunID, RootRunID: rootRunID}
	capabilities := ownCapabilities
	if lineage.IsChild() {
		if ownCapabilities != "" {
			return rundomain.Run{}, fmt.Errorf("child run %q stores capabilities of its own", id)
		}
		if !rootCapabilities.Valid {
			return rundomain.Run{}, fmt.Errorf(
				"child run %q references missing root %q",
				id,
				rootRunID,
			)
		}
		capabilities = rootCapabilities.String
	}
	capabilitiesValue, err := decodeRunCapabilities(capabilities)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q model selection: %w", id, err)
	}
	decodedUsage, err := decodeRunUsage(usage)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q usage: %w", id, err)
	}
	metrics, err := rundomain.NewMetrics(decodedUsage, steps, time.Duration(durationNs))
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("decode run %q metrics: %w", id, err)
	}
	snapshot := rundomain.Snapshot{
		SessionID: sessionID, ID: id, Lineage: lineage, ModelSelection: selection,
		GoalIncarnationID: goalIncarnationID, ActiveSegmentID: activeSegmentID, Detail: detail,
		Metrics: metrics, ContextTokens: contextTokens,
		Limits: limits, Capabilities: capabilitiesValue,
		CreatedAt: time.Unix(0, startedAt).UTC(), UpdatedAt: time.Unix(0, updatedAt).UTC(),
		MessageMark: messageMark,
	}

	switch coarse {
	case runStateRunning:
		snapshot.State = rundomain.Running
	case runStateWaiting:
		snapshot.State = rundomain.Waiting
		// Every suspended Run must join its root-owned pending set. Only interrupts
		// raised by this Run are projected onto it; an empty filtered result means
		// the Run was suspended by another source in the tree.
		if !interruptsSuspended.Valid {
			if pendingPolicy == requirePendingSet {
				return rundomain.Run{}, fmt.Errorf("run %q is waiting with no root-owned Pending set", id)
			}
			break
		}
		treeInterrupts, decodeErr := decodeInterrupts(interruptsSuspended.String)
		if decodeErr != nil {
			return rundomain.Run{}, fmt.Errorf("decode run %q interrupts: %w", id, decodeErr)
		}
		_ = treeInterrupts
	case runStateTerminal:
		reason, ok := rundomain.ParseOutcome(outcome)
		if !ok {
			return rundomain.Run{}, fmt.Errorf("run %q has unknown outcome %q", id, outcome)
		}
		state, ok := rundomain.Running.Terminate(reason)
		if !ok {
			return rundomain.Run{}, fmt.Errorf("run %q outcome %s reaches no terminal state", id, reason)
		}
		snapshot.State = state
		snapshot.Outcome = &reason
		snapshot.FinishedAt = time.Unix(0, finishedAt).UTC()
		if snapshot.Failure, err = decodeRunFailure(problem); err != nil {
			return rundomain.Run{}, fmt.Errorf("decode run %q failure: %w", id, err)
		}
	default:
		return rundomain.Run{}, fmt.Errorf("run %q has unknown state %q", id, coarse)
	}
	value, err := rundomain.Restore(snapshot)
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("run %q: %w", id, err)
	}
	return value, nil
}

func decodeRunUsage(encoded string) (*accounting.Usage, error) {
	if encoded == "" {
		return nil, nil
	}
	var row runUsageRow
	if err := json.Unmarshal([]byte(encoded), &row); err != nil {
		return nil, err
	}
	return row.usage(), nil
}

func (r *runUsageRow) usage() *accounting.Usage {
	if r == nil {
		return nil
	}
	usage := &accounting.Usage{Total: accounting.Totals{
		InputTokens:      r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CacheReadTokens:  r.CacheReadTokens,
		CacheWriteTokens: r.CacheWriteTokens,
		ReasoningTokens:  r.ReasoningTokens,
		CostUSD:          r.CostUSD,
	}}
	if len(r.ByModel) > 0 {
		usage.ByModel = make(map[string]accounting.Totals, len(r.ByModel))
		for model, perModel := range r.ByModel {
			usage.ByModel[model] = accounting.Totals{
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

func decodeRunFailure(encoded string) (*rundomain.Failure, error) {
	if encoded == "" {
		return nil, nil
	}
	var row runProblemRow
	if err := json.Unmarshal([]byte(encoded), &row); err != nil {
		return nil, err
	}
	if !row.Kind.Valid() {
		return nil, fmt.Errorf("unknown run failure kind %q", row.Kind)
	}
	return &rundomain.Failure{
		Kind:       row.Kind,
		Detail:     row.Detail,
		DocURL:     row.DocURL,
		RetryAfter: time.Duration(row.RetryAfterSeconds) * time.Second,
	}, nil
}
