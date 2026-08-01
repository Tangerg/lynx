package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

// ProcessStore persists complete executor process trees. Payload interpretation
// belongs exclusively to the execution adapter; this store never derives
// product Sessions or Runs from executor identity.
type ProcessStore struct {
	db *sql.DB
}

// NewProcessStore binds the process store to a database opened via [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

type storedProcessState struct {
	id        string
	parentID  string
	payload   []byte
	startedAt int64
}

type processUsageWire struct {
	Models []processModelUsageWire `json:"models"`
}

type processScopeWire struct {
	SessionID   string `json:"session_id"`
	Cwd         string `json:"cwd"`
	Isolated    bool   `json:"isolated"`
	GoalLeaseID string `json:"goal_lease_id"`
}

type processBudgetWire struct {
	MaxTokens  int64   `json:"max_tokens"`
	MaxCostUSD float64 `json:"max_cost_usd"`
	MaxSteps   int     `json:"max_steps"`
}

type processPolicyWire struct {
	Scope    processScopeWire  `json:"scope"`
	Provider string            `json:"provider"`
	Budget   processBudgetWire `json:"budget"`
}

type processModelUsageWire struct {
	Model            string  `json:"model"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Calls            int     `json:"calls"`
}

// SaveTree replaces the durable subtree rooted at tree.RootID. Stale
// descendants disappear before the complete new tree commits atomically.
// session_id is App-owned checkpoint metadata used only for aggregate cleanup;
// it never turns executor children into product Sessions.
func (s *ProcessStore) SaveTree(
	ctx context.Context,
	tree execution.ProcessTreeState,
	checkpoint execution.ProcessCheckpoint,
) error {
	if err := tree.Validate(); err != nil {
		return fmt.Errorf("sqlite: save process tree: %w", err)
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("sqlite: save process tree checkpoint: %w", err)
	}
	encodedPolicy, err := encodeProcessPolicy(checkpoint)
	if err != nil {
		return fmt.Errorf("sqlite: encode process tree policy: %w", err)
	}
	encodedUsage, err := encodeProcessUsage(checkpoint.Usage)
	if err != nil {
		return fmt.Errorf("sqlite: encode process tree usage: %w", err)
	}
	states := make([]storedProcessState, len(tree.Processes))
	for index, process := range tree.Processes {
		states[index] = storedProcessState{
			id:        process.ID,
			parentID:  process.ParentID,
			payload:   append([]byte(nil), process.Payload...),
			startedAt: process.StartedAt.UnixNano(),
		}
	}
	// Process ID is a total order, which is all this needs: rows carry no
	// referential constraint, so the sort exists to make one concurrent capture
	// produce one byte-identical write sequence.
	slices.SortFunc(states, func(left, right storedProcessState) int {
		return cmp.Compare(left.id, right.id)
	})

	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		committedAt := time.Now().UTC().UnixNano()
		if err := s.deleteTree(ctx, tree.RootID); err != nil {
			return err
		}
		for index, process := range states {
			var sessionID, policyData, usageData string
			if process.id == tree.RootID {
				sessionID = checkpoint.Scope.SessionID
				policyData = string(encodedPolicy)
				usageData = string(encodedUsage)
			}
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`INSERT INTO process_states(id, parent_id, session_id, started_at, build_id, payload, policy, usage, committed_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				process.id,
				process.parentID,
				sessionID,
				process.startedAt,
				checkpoint.BuildID,
				process.payload,
				policyData,
				usageData,
				committedAt,
			); err != nil {
				return fmt.Errorf("sqlite: save process states[%d] %q: %w", index, process.id, err)
			}
		}

		return nil
	})
}

// LoadTree returns the complete durable subtree and its application-owned
// checkpoint metadata from one committed tree version.
func (s *ProcessStore) LoadTree(ctx context.Context, rootID string) (execution.ProcessTreeState, execution.ProcessCheckpoint, error) {
	if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, errors.New("sqlite: load process tree: invalid root ID")
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`WITH RECURSIVE process_tree(id, parent_id, started_at, payload, build_id, policy, usage) AS (
			SELECT id, parent_id, started_at, payload, build_id, policy, usage FROM process_states WHERE id = ?
			UNION
			SELECT child.id, child.parent_id, child.started_at, child.payload, child.build_id, child.policy, child.usage
			FROM process_states AS child
			JOIN process_tree AS parent ON child.parent_id = parent.id
		)
		SELECT id, parent_id, started_at, payload, build_id, policy, usage FROM process_tree ORDER BY id`,
		rootID,
	)
	if err != nil {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	defer rows.Close()

	var processes []execution.ProcessState
	var buildID string
	var policy execution.ProcessCheckpoint
	var usage accounting.Snapshot
	metadataLoaded := false
	for rows.Next() {
		var id, parentID, rowBuildID, policyData, usageData string
		var payload []byte
		var startedAt int64
		if err := rows.Scan(&id, &parentID, &startedAt, &payload, &rowBuildID, &policyData, &usageData); err != nil {
			return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: scan process tree %q: %w", rootID, err)
		}
		if strings.TrimSpace(rowBuildID) == "" || rowBuildID != strings.TrimSpace(rowBuildID) {
			return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf(
				"sqlite: process tree %q has invalid build identity: %w",
				rootID,
				execution.ErrInvalidProcessTreeState,
			)
		}
		if buildID == "" {
			buildID = rowBuildID
		} else if rowBuildID != buildID {
			return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf(
				"sqlite: process tree %q mixes build identities %q and %q: %w",
				rootID,
				buildID,
				rowBuildID,
				execution.ErrInvalidProcessTreeState,
			)
		}
		if id == rootID {
			if policyData == "" || usageData == "" {
				return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: process tree %q has incomplete checkpoint metadata: %w", rootID, execution.ErrInvalidProcessTreeState)
			}
			decodedPolicy, err := decodeProcessPolicy(policyData)
			if err != nil {
				return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: parse process tree %q policy: %w: %w", rootID, execution.ErrInvalidProcessTreeState, err)
			}
			decodedUsage, err := decodeProcessUsage(usageData)
			if err != nil {
				return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: parse process tree %q usage: %w: %w", rootID, execution.ErrInvalidProcessTreeState, err)
			}
			policy = decodedPolicy
			usage = decodedUsage
			metadataLoaded = true
		} else if policyData != "" || usageData != "" {
			return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: descendant process state %q carries root checkpoint metadata: %w", id, execution.ErrInvalidProcessTreeState)
		}
		processes = append(processes, execution.ProcessState{
			ID:        id,
			ParentID:  parentID,
			StartedAt: time.Unix(0, startedAt).UTC(),
			Payload:   append([]byte(nil), payload...),
		})
	}
	if err := rows.Err(); err != nil {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	if len(processes) == 0 {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, execution.ErrProcessStateNotFound)
	}
	if !metadataLoaded {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: process tree %q has no checkpoint metadata: %w", rootID, execution.ErrInvalidProcessTreeState)
	}
	tree := execution.ProcessTreeState{RootID: rootID, Processes: processes}
	if err := tree.Validate(); err != nil {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	checkpoint := policy
	checkpoint.BuildID = buildID
	checkpoint.Usage = usage
	if err := checkpoint.Validate(); err != nil {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q checkpoint: %w: %w", rootID, execution.ErrInvalidProcessTreeState, err)
	}
	return tree, checkpoint, nil
}

func encodeProcessPolicy(checkpoint execution.ProcessCheckpoint) ([]byte, error) {
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(processPolicyWire{
		Scope: processScopeWire{
			SessionID:   checkpoint.Scope.SessionID,
			Cwd:         checkpoint.Scope.Cwd,
			Isolated:    checkpoint.Scope.Isolated,
			GoalLeaseID: checkpoint.Scope.GoalLeaseID,
		},
		Provider: checkpoint.Provider,
		Budget: processBudgetWire{
			MaxTokens:  checkpoint.Budget.MaxTokens,
			MaxCostUSD: checkpoint.Budget.MaxCostUSD,
			MaxSteps:   checkpoint.Budget.MaxSteps,
		},
	})
}

func decodeProcessPolicy(data string) (execution.ProcessCheckpoint, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire processPolicyWire
	if err := decoder.Decode(&wire); err != nil {
		return execution.ProcessCheckpoint{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return execution.ProcessCheckpoint{}, errors.New("policy has a trailing JSON value")
		}
		return execution.ProcessCheckpoint{}, fmt.Errorf("policy trailing JSON: %w", err)
	}
	scope := execution.TurnScope{
		SessionID:   wire.Scope.SessionID,
		Cwd:         wire.Scope.Cwd,
		Isolated:    wire.Scope.Isolated,
		GoalLeaseID: wire.Scope.GoalLeaseID,
	}
	if err := scope.Validate(); err != nil {
		return execution.ProcessCheckpoint{}, err
	}
	budget := accounting.Budget{
		MaxTokens:  wire.Budget.MaxTokens,
		MaxCostUSD: wire.Budget.MaxCostUSD,
		MaxSteps:   wire.Budget.MaxSteps,
	}
	if err := budget.Validate(); err != nil {
		return execution.ProcessCheckpoint{}, err
	}
	if wire.Provider != strings.TrimSpace(wire.Provider) {
		return execution.ProcessCheckpoint{}, errors.New("policy provider has surrounding whitespace")
	}
	return execution.ProcessCheckpoint{
		Scope:    scope,
		Provider: wire.Provider,
		Budget:   budget,
	}, nil
}

func encodeProcessUsage(usage accounting.Snapshot) ([]byte, error) {
	wire := processUsageWire{Models: make([]processModelUsageWire, len(usage.Models))}
	for index, model := range usage.Models {
		wire.Models[index] = processModelUsageWire{
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

func decodeProcessUsage(data string) (accounting.Snapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire processUsageWire
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

// DeleteTrees removes complete durable subtrees in one transaction. Unknown
// roots are already absent and therefore succeed.
func (s *ProcessStore) DeleteTrees(ctx context.Context, rootIDs []string) error {
	if len(rootIDs) == 0 {
		return errors.New("sqlite: delete process trees: no roots")
	}
	seen := make(map[string]struct{}, len(rootIDs))
	for _, rootID := range rootIDs {
		if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
			return fmt.Errorf("sqlite: delete process trees: invalid root ID %q", rootID)
		}
		if _, duplicate := seen[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete process trees: duplicate root ID %q", rootID)
		}
		seen[rootID] = struct{}{}
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, rootID := range rootIDs {
			if err := s.deleteTree(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteSessionTrees removes every checkpoint aggregate owned by sessionID.
// Root ownership is indexed as App metadata; child rows are reached through the
// executor tree relation and are never interpreted as product identities.
func (s *ProcessStore) DeleteSessionTrees(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(sessionID) != sessionID {
		return errors.New("sqlite: delete session process trees: invalid session ID")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		rootIDs, err := s.queryProcessTreeRootIDs(ctx,
			`SELECT id FROM process_states WHERE parent_id = '' AND session_id = ? ORDER BY id`,
			sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list process trees for session %q: %w", sessionID, err)
		}
		for _, rootID := range rootIDs {
			if err := s.deleteTree(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteUnownedTrees removes checkpoint aggregates that are not in keepRootIDs.
// Boot reconciliation calls it after proving the exact set of interrupted Run
// trees that still own resumable continuations.
func (s *ProcessStore) DeleteUnownedTrees(ctx context.Context, keepRootIDs []string) error {
	keep := make(map[string]struct{}, len(keepRootIDs))
	for _, rootID := range keepRootIDs {
		if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
			return fmt.Errorf("sqlite: delete unowned process trees: invalid preserved root ID %q", rootID)
		}
		if _, duplicate := keep[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete unowned process trees: duplicate preserved root ID %q", rootID)
		}
		keep[rootID] = struct{}{}
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		rootIDs, err := s.queryProcessTreeRootIDs(ctx,
			`SELECT id FROM process_states WHERE parent_id = '' ORDER BY id`,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list process tree roots: %w", err)
		}
		var stale []string
		for _, rootID := range rootIDs {
			if _, preserved := keep[rootID]; !preserved {
				stale = append(stale, rootID)
			}
		}
		for _, rootID := range stale {
			if err := s.deleteTree(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ProcessStore) queryProcessTreeRootIDs(
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
			return nil, fmt.Errorf("scan process tree root: %w", err)
		}
		rootIDs = append(rootIDs, rootID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close process tree roots: %w", err)
	}
	return rootIDs, nil
}

func (s *ProcessStore) deleteTree(ctx context.Context, rootID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`WITH RECURSIVE process_tree(id) AS (
			SELECT id FROM process_states WHERE id = ?
			UNION
			SELECT child.id
			FROM process_states AS child
			JOIN process_tree AS parent ON child.parent_id = parent.id
		)
		DELETE FROM process_states WHERE id IN (SELECT id FROM process_tree)`,
		rootID,
	); err != nil {
		return fmt.Errorf("sqlite: delete process tree %q: %w", rootID, err)
	}
	return nil
}
