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

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ProcessStore persists complete Agent process trees and delegated product
// session lineage in one SQLite transaction.
type ProcessStore struct {
	db *sql.DB
}

// NewProcessStore binds the process store to a database opened via [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

type encodedProcessSnapshot struct {
	id        string
	parentID  string
	data      []byte
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
// descendants disappear before the complete new tree and its delegated session
// lineage commit atomically.
func (s *ProcessStore) SaveTree(
	ctx context.Context,
	tree core.ProcessSnapshotTree,
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
	encoded := make([]encodedProcessSnapshot, len(tree.Snapshots))
	for index, snapshot := range tree.Snapshots {
		data, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("sqlite: encode process snapshots[%d]: %w", index, err)
		}
		encoded[index] = encodedProcessSnapshot{
			id:        snapshot.ID,
			parentID:  snapshot.ParentID,
			data:      data,
			startedAt: snapshot.StartedAt.UnixNano(),
		}
	}
	// Process ID is a total order, which is all this needs: rows carry no
	// referential constraint, so the sort exists to make one concurrent capture
	// produce one byte-identical write sequence.
	slices.SortFunc(encoded, func(left, right encodedProcessSnapshot) int {
		return cmp.Compare(left.id, right.id)
	})

	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		committedAt := time.Now().UTC().UnixNano()
		if err := s.deleteTree(ctx, tree.RootID); err != nil {
			return err
		}
		for index, snapshot := range encoded {
			var policyData, usageData string
			if snapshot.id == tree.RootID {
				policyData = string(encodedPolicy)
				usageData = string(encodedUsage)
			}
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`INSERT INTO process_snapshots(id, parent_id, build_id, snapshot, policy, usage, committed_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				snapshot.id,
				snapshot.parentID,
				checkpoint.BuildID,
				string(snapshot.data),
				policyData,
				usageData,
				committedAt,
			); err != nil {
				return fmt.Errorf("sqlite: save process snapshots[%d] %q: %w", index, snapshot.id, err)
			}
		}

		if checkpoint.Scope.SessionID == "" || len(encoded) == 1 {
			return nil
		}

		sessions := NewSessionStore(s.db)
		if _, err := sessions.Get(ctx, checkpoint.Scope.SessionID); err != nil {
			return fmt.Errorf("sqlite: save process lineage: session %q: %w", checkpoint.Scope.SessionID, err)
		}
		for _, snapshot := range encoded {
			if snapshot.id == tree.RootID {
				continue
			}
			parentID := snapshot.parentID
			if parentID == tree.RootID {
				parentID = checkpoint.Scope.SessionID
			}
			if _, err := sessions.SaveSubtask(ctx, session.Subtask{
				ID:        snapshot.id,
				ParentID:  parentID,
				StartedAt: time.Unix(0, snapshot.startedAt).UTC(),
				UpdatedAt: time.Unix(0, committedAt).UTC(),
			}); err != nil {
				return fmt.Errorf("sqlite: save process subtask %q: %w", snapshot.id, err)
			}
		}
		return nil
	})
}

// LoadTree returns the complete durable subtree and its application-owned
// checkpoint metadata from one committed tree version.
func (s *ProcessStore) LoadTree(ctx context.Context, rootID string) (core.ProcessSnapshotTree, execution.ProcessCheckpoint, error) {
	if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, errors.New("sqlite: load process tree: invalid root ID")
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`WITH RECURSIVE process_tree(id, snapshot, build_id, policy, usage) AS (
			SELECT id, snapshot, build_id, policy, usage FROM process_snapshots WHERE id = ?
			UNION
			SELECT child.id, child.snapshot, child.build_id, child.policy, child.usage
			FROM process_snapshots AS child
			JOIN process_tree AS parent ON child.parent_id = parent.id
		)
		SELECT id, snapshot, build_id, policy, usage FROM process_tree ORDER BY id`,
		rootID,
	)
	if err != nil {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	defer rows.Close()

	var snapshots []core.ProcessSnapshot
	var buildID string
	var policy execution.ProcessCheckpoint
	var usage accounting.Snapshot
	metadataLoaded := false
	for rows.Next() {
		var id, data, rowBuildID, policyData, usageData string
		if err := rows.Scan(&id, &data, &rowBuildID, &policyData, &usageData); err != nil {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: scan process tree %q: %w", rootID, err)
		}
		if strings.TrimSpace(rowBuildID) == "" || rowBuildID != strings.TrimSpace(rowBuildID) {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf(
				"sqlite: process tree %q has invalid build identity: %w",
				rootID,
				core.ErrInvalidSnapshot,
			)
		}
		if buildID == "" {
			buildID = rowBuildID
		} else if rowBuildID != buildID {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf(
				"sqlite: process tree %q mixes build identities %q and %q: %w",
				rootID,
				buildID,
				rowBuildID,
				core.ErrInvalidSnapshot,
			)
		}
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: parse snapshot %q: %w: %w", id, core.ErrInvalidSnapshot, err)
		}
		if snapshot.ID != id {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: snapshot ID %q does not match row %q: %w", snapshot.ID, id, core.ErrInvalidSnapshot)
		}
		if id == rootID {
			if policyData == "" || usageData == "" {
				return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: process tree %q has incomplete checkpoint metadata: %w", rootID, core.ErrInvalidSnapshot)
			}
			decodedPolicy, err := decodeProcessPolicy(policyData)
			if err != nil {
				return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: parse process tree %q policy: %w: %w", rootID, core.ErrInvalidSnapshot, err)
			}
			decodedUsage, err := decodeProcessUsage(usageData)
			if err != nil {
				return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: parse process tree %q usage: %w: %w", rootID, core.ErrInvalidSnapshot, err)
			}
			policy = decodedPolicy
			usage = decodedUsage
			metadataLoaded = true
		} else if policyData != "" || usageData != "" {
			return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: descendant snapshot %q carries root checkpoint metadata: %w", id, core.ErrInvalidSnapshot)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	if len(snapshots) == 0 {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, execution.ErrProcessSnapshotNotFound)
	}
	if !metadataLoaded {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: process tree %q has no checkpoint metadata: %w", rootID, core.ErrInvalidSnapshot)
	}
	tree := core.ProcessSnapshotTree{RootID: rootID, Snapshots: snapshots}
	if err := tree.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q: %w", rootID, err)
	}
	checkpoint := policy
	checkpoint.BuildID = buildID
	checkpoint.Usage = usage
	if err := checkpoint.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, execution.ProcessCheckpoint{}, fmt.Errorf("sqlite: load process tree %q checkpoint: %w: %w", rootID, core.ErrInvalidSnapshot, err)
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

func (s *ProcessStore) deleteTree(ctx context.Context, rootID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`WITH RECURSIVE process_tree(id) AS (
			SELECT id FROM process_snapshots WHERE id = ?
			UNION
			SELECT child.id
			FROM process_snapshots AS child
			JOIN process_tree AS parent ON child.parent_id = parent.id
		)
		DELETE FROM process_snapshots WHERE id IN (SELECT id FROM process_tree)`,
		rootID,
	); err != nil {
		return fmt.Errorf("sqlite: delete process tree %q: %w", rootID, err)
	}
	return nil
}
