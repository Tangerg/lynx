package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type sessionSnapshotRecord struct {
	Session      sessionFrame      `json:"session"`
	Transcript   []blockFrame      `json:"transcript"`
	Runs         []runFrame        `json:"runs"`
	Plan         planSnapshotFrame `json:"plan"`
	Interactions []interactionJSON `json:"interactions,omitempty"`
}

type sessionFrame struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Model     string    `json:"model,omitempty"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	Favorite  bool      `json:"favorite,omitempty"`
	Revision  uint64    `json:"revision"`
}

type runFrame struct {
	ID               string           `json:"id"`
	SessionID        string           `json:"sessionId"`
	SpawnedByBlockID string           `json:"spawnedByBlockId,omitempty"`
	ParentRunID      string           `json:"parentRunId,omitempty"`
	RootRunID        string           `json:"rootRunId,omitempty"`
	Provider         string           `json:"provider,omitempty"`
	Model            string           `json:"model,omitempty"`
	Status           string           `json:"status"`
	ActiveSegmentID  string           `json:"activeSegmentId,omitempty"`
	CreatedAt        time.Time        `json:"createdAt,omitzero"`
	FinishedAt       time.Time        `json:"finishedAt,omitzero"`
	Limits           *runLimitsFrame  `json:"limits,omitempty"`
	Outcome          *outcomeJSON     `json:"outcome,omitempty"`
	Usage            usageJSON        `json:"usage"`
	ProtocolProfile  *runContractJSON `json:"protocolProfile,omitempty"`
}

type runLimitsFrame struct {
	MaxTotalTokens int64   `json:"maxTotalTokens,omitempty"`
	MaxSteps       int     `json:"maxSteps,omitempty"`
	MaxBudgetUSD   float64 `json:"maxBudgetUsd,omitempty"`
}

type runContractJSON struct {
	RequiredFeatures []string `json:"requiredFeatures"`
	InterruptTypes   []string `json:"interruptTypes"`
}

type planSnapshotFrame struct {
	Revision uint64      `json:"revision"`
	Items    []planFrame `json:"items"`
}

type runPageRecord struct {
	Items      []runFrame `json:"items"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type runCancellationRecord struct {
	Canceled runFrame `json:"canceled"`
	Root     runFrame `json:"root"`
}

// WriteSessionSnapshotJSON writes the CLI's stable cold-read JSON projection.
// Domain values intentionally carry no encoding tags, so this adapter owns the
// external field names instead of leaking a delivery format into the core.
func WriteSessionSnapshotJSON(w io.Writer, snapshot agent.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("render session snapshot: %w", err)
	}
	record := sessionSnapshotRecord{
		Session: sessionFrame{
			ID: snapshot.Session.ID, Title: snapshot.Session.Title, Status: string(snapshot.Session.Status),
			Model: snapshot.Session.Model, Workspace: snapshot.Session.Workspace,
			CreatedAt: snapshot.Session.CreatedAt, UpdatedAt: snapshot.Session.UpdatedAt,
			Favorite: snapshot.Session.Favorite, Revision: snapshot.Session.Revision,
		},
		Transcript:   make([]blockFrame, 0, len(snapshot.Transcript)),
		Runs:         make([]runFrame, 0, len(snapshot.Runs)),
		Plan:         planSnapshotFrame{Revision: snapshot.PlanRevision, Items: encodePlan(snapshot.Plan)},
		Interactions: encodeInteractions(snapshot.Interactions),
	}
	for _, block := range snapshot.Transcript {
		record.Transcript = append(record.Transcript, *encodeBlock(block))
	}
	for _, run := range snapshot.Runs {
		record.Runs = append(record.Runs, encodeRun(run))
	}
	return json.NewEncoder(w).Encode(record)
}

// WriteRunJSON writes one durable run projection using the same field contract
// as runs embedded in a session snapshot.
func WriteRunJSON(w io.Writer, run agent.Run) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("render run: %w", err)
	}
	return json.NewEncoder(w).Encode(encodeRun(run))
}

func WriteRunPageJSON(w io.Writer, page agent.RunPage) error {
	if err := page.Validate(); err != nil {
		return fmt.Errorf("render run page: %w", err)
	}
	record := runPageRecord{Items: make([]runFrame, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, run := range page.Items {
		record.Items = append(record.Items, encodeRun(run))
	}
	return json.NewEncoder(w).Encode(record)
}

func WriteRunCancellationJSON(w io.Writer, result agent.RunCancellation) error {
	if err := result.Validate(); err != nil {
		return fmt.Errorf("render run cancellation: %w", err)
	}
	return json.NewEncoder(w).Encode(runCancellationRecord{
		Canceled: encodeRun(result.Canceled),
		Root:     encodeRun(result.Root),
	})
}

func encodeRun(run agent.Run) runFrame {
	encoded := runFrame{
		ID: run.ID, SessionID: run.SessionID,
		SpawnedByBlockID: run.Lineage.SpawnedByBlockID,
		ParentRunID:      run.Lineage.ParentRunID, RootRunID: run.Lineage.RootRunID,
		Provider: run.Provider, Model: run.Model,
		Status: string(run.Status), ActiveSegmentID: run.ActiveSegmentID,
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		Usage: *encodeUsage(run.Usage),
	}
	if run.Limits != (agent.RunLimits{}) {
		encoded.Limits = &runLimitsFrame{MaxTotalTokens: run.Limits.MaxTotalTokens, MaxSteps: run.Limits.MaxSteps, MaxBudgetUSD: run.Limits.MaxBudgetUSD}
	}
	if run.Status == agent.RunStatusFinished {
		encoded.Outcome = encodeOutcome(run.Outcome)
	}
	if run.Contract != nil {
		encoded.ProtocolProfile = &runContractJSON{
			RequiredFeatures: make([]string, len(run.Contract.RequiredFeatures)),
			InterruptTypes:   make([]string, len(run.Contract.InteractionKinds)),
		}
		for index, feature := range run.Contract.RequiredFeatures {
			encoded.ProtocolProfile.RequiredFeatures[index] = string(feature)
		}
		for index, kind := range run.Contract.InteractionKinds {
			encoded.ProtocolProfile.InterruptTypes[index] = string(kind)
		}
	}
	return encoded
}
