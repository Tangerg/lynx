package sessions

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ErrInvalidPortableSnapshot marks a structurally decoded archive that cannot
// satisfy the product aggregate invariants. Delivery maps this use-case error
// to invalid_params without reimplementing the validation itself.
var ErrInvalidPortableSnapshot = errors.New("sessions: invalid portable snapshot")

// PortableSnapshot is the transport-neutral, terminal-only session archive
// accepted by restore. It deliberately separates a portable run's outcome from
// its derived lifecycle state: restore owns rebuilding that state machine.
type PortableSnapshot struct {
	Session     PortableSession
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []PortableRun
	ToolResults []offload.ToolResultBlob
}

// PortableSession is the terminal archive identity. It intentionally excludes
// live aggregate details such as lineage, kind, isolation, and revision: an
// imported archive is always admitted as a standalone conversation.
type PortableSession struct {
	ID        string
	Title     string
	Cwd       string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
}

// PortableRun is one terminal run in a portable snapshot. State is not carried
// because it is derived from Outcome by the execution state machine.
type PortableRun struct {
	SessionID       string
	ID              string
	SpawnedByItemID string
	Provider        string
	Model           string
	Outcome         execution.Outcome
	Result          *transcript.RunResult
	Detail          string
	CreatedAt       time.Time
	FinishedAt      time.Time
	UpdatedAt       time.Time
	MessageMark     int
}

// CanonicalSnapshot rebuilds and validates the canonical aggregate from a
// portable archive. Protocol adapters only decode their wire document into
// PortableSnapshot values; the restore use case owns its one normalization.
func (p PortableSnapshot) CanonicalSnapshot() (Snapshot, error) {
	snapshot := Snapshot{
		Session:     p.Session.session(),
		Messages:    p.Messages,
		Items:       append([]transcript.Item(nil), p.Items...),
		ToolResults: append([]offload.ToolResultBlob(nil), p.ToolResults...),
		Runs:        make([]transcript.Run, 0, len(p.Runs)),
	}
	for _, portable := range p.Runs {
		state, ok := execution.Running.Terminate(portable.Outcome)
		if !ok {
			return Snapshot{}, fmt.Errorf("%w: run %q has invalid outcome %s", ErrInvalidPortableSnapshot, portable.ID, portable.Outcome)
		}
		outcome := portable.Outcome
		if portable.Result == nil {
			return Snapshot{}, fmt.Errorf("%w: run %q has no result", ErrInvalidPortableSnapshot, portable.ID)
		}
		snapshot.Runs = append(snapshot.Runs, transcript.Run{
			SessionID:       portable.SessionID,
			ID:              portable.ID,
			SpawnedByItemID: portable.SpawnedByItemID,
			Provider:        portable.Provider,
			Model:           portable.Model,
			State:           state,
			Outcome:         &outcome,
			Result:          portable.Result,
			Detail:          portable.Detail,
			CreatedAt:       portable.CreatedAt,
			FinishedAt:      portable.FinishedAt,
			UpdatedAt:       portable.UpdatedAt,
			MessageMark:     portable.MessageMark,
		})
	}
	if err := bindPortableToolResults(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %w", ErrInvalidPortableSnapshot, err)
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %w", ErrInvalidPortableSnapshot, err)
	}
	return snapshot, nil
}

func (p PortableSession) session() session.Session {
	return session.Session{
		ID:        p.ID,
		Title:     p.Title,
		Cwd:       p.Cwd,
		Model:     p.Model,
		StartedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		Favorite:  p.Favorite,
	}
}

func bindPortableToolResults(snapshot *Snapshot) error {
	items := make(map[string]int, len(snapshot.Items))
	for index, item := range snapshot.Items {
		if _, duplicate := items[item.ID]; duplicate {
			return fmt.Errorf("sessions: portable snapshot contains duplicate item %q", item.ID)
		}
		items[item.ID] = index
	}
	for _, blob := range snapshot.ToolResults {
		index, found := items[blob.ItemID]
		if !found {
			return fmt.Errorf("sessions: portable tool result %q references unknown item %q", blob.ID, blob.ItemID)
		}
		item := &snapshot.Items[index]
		if item.Tool == nil {
			return fmt.Errorf("sessions: portable tool result %q references non-tool item %q", blob.ID, item.ID)
		}
		invocation := *item.Tool
		invocation.Offload = &offload.Ref{ID: blob.ID}
		item.Tool = &invocation
	}
	return nil
}

// PortableSnapshot returns the normalized, terminal-only representation used by
// an archive encoder. It keeps archive projection out of Delivery while leaving
// the selected wire format to the protocol adapter.
func (s Snapshot) PortableSnapshot() (PortableSnapshot, error) {
	normalized, err := s.NormalizeForRestore()
	if err != nil {
		return PortableSnapshot{}, err
	}
	portable := PortableSnapshot{
		Session: PortableSession{
			ID:        normalized.Session.ID,
			Title:     normalized.Session.Title,
			Cwd:       normalized.Session.Cwd,
			Model:     normalized.Session.Model,
			CreatedAt: normalized.Session.StartedAt,
			UpdatedAt: normalized.Session.UpdatedAt,
			Favorite:  normalized.Session.Favorite,
		},
		Messages:    normalized.Messages,
		Items:       normalized.Items,
		ToolResults: normalized.ToolResults,
		Runs:        make([]PortableRun, 0, len(normalized.Runs)),
	}
	for _, run := range normalized.Runs {
		if run.Outcome == nil || run.Result == nil {
			return PortableSnapshot{}, fmt.Errorf("sessions: terminal run %q has no outcome or result", run.ID)
		}
		portable.Runs = append(portable.Runs, PortableRun{
			SessionID:       run.SessionID,
			ID:              run.ID,
			SpawnedByItemID: run.SpawnedByItemID,
			Provider:        run.Provider,
			Model:           run.Model,
			Outcome:         *run.Outcome,
			Result:          run.Result,
			Detail:          run.Detail,
			CreatedAt:       run.CreatedAt,
			FinishedAt:      run.FinishedAt,
			UpdatedAt:       run.UpdatedAt,
			MessageMark:     run.MessageMark,
		})
	}
	return portable, nil
}
