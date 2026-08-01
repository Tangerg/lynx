package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// ExecutorCheckpointStore persists one opaque checkpoint aggregate per
// executor tree root.
// Payload interpretation and all child-process topology belong exclusively to
// the execution adapter.
type ExecutorCheckpointStore struct {
	db *sql.DB
}

// NewExecutorCheckpointStore binds opaque executor checkpoint persistence to a
// database opened via [Open].
func NewExecutorCheckpointStore(db *sql.DB) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{db: db}
}

type executorUsageWire struct {
	Models []executorModelUsageWire `json:"models"`
}

type executorScopeWire struct {
	SessionID   string `json:"session_id"`
	Cwd         string `json:"cwd"`
	Isolated    bool   `json:"isolated"`
	GoalLeaseID string `json:"goal_lease_id"`
}

type executorLimitsWire struct {
	MaxTotalTokens int64   `json:"max_total_tokens"`
	MaxBudgetUSD   float64 `json:"max_budget_usd"`
	MaxSteps       int     `json:"max_steps"`
}

type executorPolicyWire struct {
	Scope    executorScopeWire  `json:"scope"`
	Provider string             `json:"provider"`
	Model    string             `json:"model"`
	Limits   executorLimitsWire `json:"limits"`
}

type executorModelUsageWire struct {
	Model            string  `json:"model"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Calls            int     `json:"calls"`
}

// SaveCheckpoint atomically advances one root-owned executor checkpoint. The
// root's Session, build, host scope, model selection, and budget are immutable;
// only the opaque payload and cumulative usage may advance between barriers.
func (s *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint execution.ExecutorCheckpoint) error {
	if err := checkpoint.ValidateOwnership(checkpoint.RootProcessID, checkpoint.Scope.SessionID); err != nil {
		return fmt.Errorf("sqlite: save executor checkpoint: %w", err)
	}
	encodedPolicy, err := encodeExecutorPolicy(checkpoint)
	if err != nil {
		return fmt.Errorf("sqlite: encode executor checkpoint policy: %w", err)
	}
	encodedUsage, err := encodeExecutorUsage(checkpoint.Usage)
	if err != nil {
		return fmt.Errorf("sqlite: encode executor checkpoint usage: %w", err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		var owner, buildID, policy, storedUsageData string
		err := conn(ctx, s.db).QueryRowContext(ctx,
			`SELECT session_id, build_id, policy, usage
			   FROM executor_checkpoints
			  WHERE root_process_id = ?`,
			checkpoint.RootProcessID,
		).Scan(&owner, &buildID, &policy, &storedUsageData)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = conn(ctx, s.db).ExecContext(ctx,
				`INSERT INTO executor_checkpoints(
					root_process_id, session_id, build_id, payload, policy, usage, committed_at
				 ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				checkpoint.RootProcessID,
				checkpoint.Scope.SessionID,
				checkpoint.BuildID,
				checkpoint.Payload,
				string(encodedPolicy),
				string(encodedUsage),
				time.Now().UTC().UnixNano(),
			)
			if err != nil {
				return fmt.Errorf("sqlite: insert executor checkpoint %q: %w", checkpoint.RootProcessID, err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("sqlite: inspect executor checkpoint %q before save: %w", checkpoint.RootProcessID, err)
		}
		switch {
		case owner != checkpoint.Scope.SessionID:
			return fmt.Errorf(
				"sqlite: executor checkpoint %q belongs to Session %q, not %q: %w",
				checkpoint.RootProcessID,
				owner,
				checkpoint.Scope.SessionID,
				execution.ErrInvalidExecutorCheckpoint,
			)
		case buildID != checkpoint.BuildID:
			return fmt.Errorf(
				"sqlite: executor checkpoint %q build is immutable: stored %q, replacement %q: %w",
				checkpoint.RootProcessID,
				buildID,
				checkpoint.BuildID,
				execution.ErrInvalidExecutorCheckpoint,
			)
		case policy != string(encodedPolicy):
			return fmt.Errorf(
				"sqlite: executor checkpoint %q policy is immutable: stored %s, replacement %s: %w",
				checkpoint.RootProcessID,
				policy,
				encodedPolicy,
				execution.ErrInvalidExecutorCheckpoint,
			)
		}
		storedUsage, err := decodeExecutorUsage(storedUsageData)
		if err != nil {
			return fmt.Errorf(
				"sqlite: decode stored executor checkpoint %q usage: %w: %w",
				checkpoint.RootProcessID,
				execution.ErrInvalidExecutorCheckpoint,
				err,
			)
		}
		if err := checkpoint.Usage.ValidateAdvanceFrom(storedUsage); err != nil {
			return fmt.Errorf(
				"sqlite: executor checkpoint %q cumulative usage cannot advance: %w: %w",
				checkpoint.RootProcessID,
				execution.ErrInvalidExecutorCheckpoint,
				err,
			)
		}
		result, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE executor_checkpoints
			    SET payload = ?, usage = ?, committed_at = ?
			  WHERE root_process_id = ?`,
			checkpoint.Payload,
			string(encodedUsage),
			time.Now().UTC().UnixNano(),
			checkpoint.RootProcessID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: advance executor checkpoint %q: %w", checkpoint.RootProcessID, err)
		}
		written, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect advanced executor checkpoint %q: %w", checkpoint.RootProcessID, err)
		}
		if written != 1 {
			return fmt.Errorf("sqlite: advance executor checkpoint %q affected %d rows", checkpoint.RootProcessID, written)
		}
		return nil
	})
}

// LoadCheckpoint returns one complete opaque executor checkpoint.
func (s *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootProcessID string) (execution.ExecutorCheckpoint, error) {
	if strings.TrimSpace(rootProcessID) == "" || strings.TrimSpace(rootProcessID) != rootProcessID {
		return execution.ExecutorCheckpoint{}, errors.New("sqlite: load executor checkpoint: invalid root process ID")
	}
	var buildID, policyData, usageData string
	var payload []byte
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT build_id, payload, policy, usage
		   FROM executor_checkpoints
		  WHERE root_process_id = ?`,
		rootProcessID,
	).Scan(&buildID, &payload, &policyData, &usageData)
	if errors.Is(err, sql.ErrNoRows) {
		return execution.ExecutorCheckpoint{}, fmt.Errorf(
			"sqlite: load executor checkpoint %q: %w",
			rootProcessID,
			execution.ErrExecutorCheckpointNotFound,
		)
	}
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootProcessID, err)
	}
	policy, err := decodeExecutorPolicy(policyData)
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf(
			"sqlite: decode executor checkpoint %q policy: %w: %w",
			rootProcessID,
			execution.ErrInvalidExecutorCheckpoint,
			err,
		)
	}
	usage, err := decodeExecutorUsage(usageData)
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf(
			"sqlite: decode executor checkpoint %q usage: %w: %w",
			rootProcessID,
			execution.ErrInvalidExecutorCheckpoint,
			err,
		)
	}
	checkpoint := policy
	checkpoint.RootProcessID = rootProcessID
	checkpoint.Payload = append([]byte(nil), payload...)
	checkpoint.BuildID = buildID
	checkpoint.Usage = usage
	if err := checkpoint.ValidateOwnership(rootProcessID, checkpoint.Scope.SessionID); err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootProcessID, err)
	}
	return checkpoint, nil
}

func encodeExecutorPolicy(checkpoint execution.ExecutorCheckpoint) ([]byte, error) {
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(executorPolicyWire{
		Scope: executorScopeWire{
			SessionID:   checkpoint.Scope.SessionID,
			Cwd:         checkpoint.Scope.Cwd,
			Isolated:    checkpoint.Scope.Isolated,
			GoalLeaseID: checkpoint.Scope.GoalLeaseID,
		},
		Provider: checkpoint.ModelSelection.Provider(),
		Model:    checkpoint.ModelSelection.Model(),
		Limits: executorLimitsWire{
			MaxTotalTokens: checkpoint.Limits.MaxTotalTokens,
			MaxBudgetUSD:   checkpoint.Limits.MaxBudgetUSD,
			MaxSteps:       checkpoint.Limits.MaxSteps,
		},
	})
}

func decodeExecutorPolicy(data string) (execution.ExecutorCheckpoint, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire executorPolicyWire
	if err := decoder.Decode(&wire); err != nil {
		return execution.ExecutorCheckpoint{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return execution.ExecutorCheckpoint{}, errors.New("policy has a trailing JSON value")
		}
		return execution.ExecutorCheckpoint{}, fmt.Errorf("policy trailing JSON: %w", err)
	}
	scope := execution.TurnScope{
		SessionID:   wire.Scope.SessionID,
		Cwd:         wire.Scope.Cwd,
		Isolated:    wire.Scope.Isolated,
		GoalLeaseID: wire.Scope.GoalLeaseID,
	}
	if err := scope.Validate(); err != nil {
		return execution.ExecutorCheckpoint{}, err
	}
	limits := execution.RunLimits{
		MaxTotalTokens: wire.Limits.MaxTotalTokens,
		MaxBudgetUSD:   wire.Limits.MaxBudgetUSD,
		MaxSteps:       wire.Limits.MaxSteps,
	}
	if err := limits.Validate(); err != nil {
		return execution.ExecutorCheckpoint{}, err
	}
	selection, err := modelref.New(wire.Provider, wire.Model)
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("policy model selection: %w", err)
	}
	if wire.Provider != strings.TrimSpace(wire.Provider) || wire.Model != strings.TrimSpace(wire.Model) {
		return execution.ExecutorCheckpoint{}, errors.New("policy model selection has surrounding whitespace")
	}
	return execution.ExecutorCheckpoint{
		Scope:          scope,
		ModelSelection: selection,
		Limits:         limits,
	}, nil
}

func encodeExecutorUsage(usage accounting.Snapshot) ([]byte, error) {
	wire := executorUsageWire{Models: make([]executorModelUsageWire, len(usage.Models))}
	for index, model := range usage.Models {
		wire.Models[index] = executorModelUsageWire{
			Model:            model.Model,
			PromptTokens:     model.PromptTokens,
			CompletionTokens: model.CompletionTokens,
			ReasoningTokens:  model.ReasoningTokens,
			CacheReadTokens:  model.CacheReadTokens,
			CacheWriteTokens: model.CacheWriteTokens,
			CostUSD:          model.CostUSD,
			Calls:            model.Calls,
		}
	}
	return json.Marshal(wire)
}

func decodeExecutorUsage(data string) (accounting.Snapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire executorUsageWire
	if err := decoder.Decode(&wire); err != nil {
		return accounting.Snapshot{}, err
	}
	if wire.Models == nil {
		return accounting.Snapshot{}, errors.New("usage models must be an array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return accounting.Snapshot{}, errors.New("usage has a trailing JSON value")
		}
		return accounting.Snapshot{}, fmt.Errorf("usage trailing JSON: %w", err)
	}
	usage := accounting.Snapshot{Models: make([]accounting.ModelUsage, len(wire.Models))}
	for index, model := range wire.Models {
		usage.Models[index] = accounting.ModelUsage{
			Model: model.Model,
			TokenUsage: accounting.TokenUsage{
				PromptTokens:     model.PromptTokens,
				CompletionTokens: model.CompletionTokens,
				ReasoningTokens:  model.ReasoningTokens,
				CacheReadTokens:  model.CacheReadTokens,
				CacheWriteTokens: model.CacheWriteTokens,
			},
			CostUSD: model.CostUSD,
			Calls:   model.Calls,
		}
	}
	if err := usage.Validate(); err != nil {
		return accounting.Snapshot{}, err
	}
	return usage, nil
}

// DeleteCheckpoints removes complete root-owned checkpoint aggregates in one
// transaction, but only when they belong to sessionID. Unknown roots are
// already absent and therefore succeed; a root owned by another Session is
// rejected as corruption rather than deleted.
func (s *ExecutorCheckpointStore) DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(sessionID) != sessionID {
		return errors.New("sqlite: delete executor checkpoints: invalid session ID")
	}
	if len(rootIDs) == 0 {
		return errors.New("sqlite: delete executor checkpoints: no roots")
	}
	seen := make(map[string]struct{}, len(rootIDs))
	for _, rootID := range rootIDs {
		if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
			return fmt.Errorf("sqlite: delete executor checkpoints: invalid root ID %q", rootID)
		}
		if _, duplicate := seen[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete executor checkpoints: duplicate root ID %q", rootID)
		}
		seen[rootID] = struct{}{}
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, rootID := range rootIDs {
			if err := s.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteSessionCheckpoints removes every checkpoint aggregate owned by
// sessionID.
func (s *ExecutorCheckpointStore) DeleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(sessionID) != sessionID {
		return errors.New("sqlite: delete session executor checkpoints: invalid session ID")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		rootIDs, err := s.queryCheckpointRootIDs(ctx,
			`SELECT root_process_id FROM executor_checkpoints WHERE session_id = ? ORDER BY root_process_id`,
			sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoints for Session %q: %w", sessionID, err)
		}
		for _, rootID := range rootIDs {
			if err := s.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteUnownedCheckpoints removes checkpoint aggregates that are not in
// keepRootIDs.
// Boot reconciliation calls it after proving the exact set of interrupted Run
// trees that still own resumable continuations.
func (s *ExecutorCheckpointStore) DeleteUnownedCheckpoints(ctx context.Context, keepRootIDs []string) error {
	keep := make(map[string]struct{}, len(keepRootIDs))
	for _, rootID := range keepRootIDs {
		if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
			return fmt.Errorf("sqlite: delete unowned executor checkpoints: invalid preserved root ID %q", rootID)
		}
		if _, duplicate := keep[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete unowned executor checkpoints: duplicate preserved root ID %q", rootID)
		}
		keep[rootID] = struct{}{}
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		rootIDs, err := s.queryCheckpointRootIDs(ctx,
			`SELECT root_process_id FROM executor_checkpoints ORDER BY root_process_id`,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoint roots: %w", err)
		}
		var stale []string
		for _, rootID := range rootIDs {
			if _, preserved := keep[rootID]; !preserved {
				stale = append(stale, rootID)
			}
		}
		for _, rootID := range stale {
			if err := s.deleteCheckpoint(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ExecutorCheckpointStore) queryCheckpointRootIDs(
	ctx context.Context,
	query string,
	args ...any,
) ([]string, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var rootIDs []string
	for rows.Next() {
		var rootID string
		if err := rows.Scan(&rootID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan executor checkpoint root: %w", err)
		}
		rootIDs = append(rootIDs, rootID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close executor checkpoint roots: %w", err)
	}
	return rootIDs, nil
}

func (s *ExecutorCheckpointStore) deleteCheckpoint(ctx context.Context, rootProcessID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM executor_checkpoints WHERE root_process_id = ?`,
		rootProcessID,
	); err != nil {
		return fmt.Errorf("sqlite: delete executor checkpoint %q: %w", rootProcessID, err)
	}
	return nil
}

func (s *ExecutorCheckpointStore) deleteOwnedCheckpoint(
	ctx context.Context,
	sessionID string,
	rootProcessID string,
) error {
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM executor_checkpoints WHERE root_process_id = ? AND session_id = ?`,
		rootProcessID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete executor checkpoint %q for Session %q: %w", rootProcessID, sessionID, err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted executor checkpoint %q: %w", rootProcessID, err)
	}
	if deleted == 1 {
		return nil
	}
	var owner string
	err = conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id FROM executor_checkpoints WHERE root_process_id = ?`,
		rootProcessID,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect executor checkpoint %q owner: %w", rootProcessID, err)
	}
	return fmt.Errorf(
		"sqlite: executor checkpoint %q belongs to Session %q, not %q: %w",
		rootProcessID,
		owner,
		sessionID,
		execution.ErrInvalidExecutorCheckpoint,
	)
}
