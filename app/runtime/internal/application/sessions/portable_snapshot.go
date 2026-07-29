package sessions

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
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
	// Todos is the session's task list, carried as a value so an archive restores
	// the work plan attached to the conversation rather than just the conversation.
	Todos []todo.Item
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
	// ParentRunID and RootRunID are the child edges. Nothing produces them while
	// subagents are off; they exist so an archive written by a build that has them
	// round-trips, and so this one refuses a broken tree instead of importing it.
	ParentRunID string
	RootRunID   string
	Provider    string
	Model       string
	Outcome     execution.Outcome
	Error       *transcript.Problem
	Metrics     transcript.RunMetrics
	Limits      execution.RunLimits
	// ProtocolProfile is a POINTER because absence is a distinct fact: an empty
	// profile is the Minimal Profile — a meaning — while a missing one is a run that
	// never said. A root must carry it; a child must not, and inherits its root's.
	ProtocolProfile *execution.RunProtocolProfile
	Detail          string
	CreatedAt       time.Time
	FinishedAt      time.Time
	UpdatedAt       time.Time
	MessageMark     int
}

// rootID is the run that owns this run's protocol contract: itself when it is a
// root, its RootRunID when it is a child.
func (p PortableRun) rootID() string {
	if p.RootRunID != "" {
		return p.RootRunID
	}
	return p.ID
}

// validateLineage checks the identity rules a schema cannot: a root carries the
// protocol contract it published under, a child carries none of its own, and a
// child's edges name other runs in the same session rather than itself.
//
// These are not shape rules — JSON Schema cannot compare two fields, and "root"
// is the ABSENCE of the child edges, which no presence rule can condition on — so
// they belong to the transaction that turns an archive into a session.
func (p PortableRun) validateLineage() error {
	child := p.SpawnedByItemID != "" || p.ParentRunID != "" || p.RootRunID != ""
	if !child {
		if p.ProtocolProfile == nil {
			return fmt.Errorf("root run %q carries no protocol profile", p.ID)
		}
		return nil
	}
	if p.ParentRunID == p.ID || p.RootRunID == p.ID {
		return fmt.Errorf("child run %q names itself as its own parent or root", p.ID)
	}
	if p.ProtocolProfile != nil {
		return fmt.Errorf("child run %q carries a protocol profile of its own", p.ID)
	}
	return nil
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
		Todos:       append([]todo.Item(nil), p.Todos...),
	}
	if err := todo.ValidateSnapshot(snapshot.Todos); err != nil {
		return Snapshot{}, fmt.Errorf("%w: todos: %w", ErrInvalidPortableSnapshot, err)
	}
	// A child reads its root's contract, so the roots' profiles are collected before
	// any run is rebuilt — the archive states each contract exactly once, and a child
	// that could carry its own would be a second statement of it.
	profiles := make(map[string]execution.RunProtocolProfile, len(p.Runs))
	for _, portable := range p.Runs {
		if portable.ProtocolProfile != nil {
			profiles[portable.ID] = *portable.ProtocolProfile
		}
	}
	for _, portable := range p.Runs {
		if err := portable.validateLineage(); err != nil {
			return Snapshot{}, fmt.Errorf("%w: %w", ErrInvalidPortableSnapshot, err)
		}
		if _, known := profiles[portable.rootID()]; !known {
			return Snapshot{}, fmt.Errorf("%w: run %q names root %q, which this archive does not contain",
				ErrInvalidPortableSnapshot, portable.ID, portable.rootID())
		}
		selection, err := modelref.New(portable.Provider, portable.Model)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: run %q model selection: %w", ErrInvalidPortableSnapshot, portable.ID, err)
		}
		state, ok := execution.Running.Terminate(portable.Outcome)
		if !ok {
			return Snapshot{}, fmt.Errorf("%w: run %q has invalid outcome %s", ErrInvalidPortableSnapshot, portable.ID, portable.Outcome)
		}
		outcome := portable.Outcome
		snapshot.Runs = append(snapshot.Runs, transcript.Run{
			SessionID:       portable.SessionID,
			ID:              portable.ID,
			SpawnedByItemID: portable.SpawnedByItemID,
			ModelSelection:  selection,
			State:           state,
			Outcome:         &outcome,
			Error:           portable.Error,
			Metrics:         portable.Metrics,
			Limits:          portable.Limits,
			ProtocolProfile: profiles[portable.rootID()],
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
		Todos:       normalized.Todos,
		Runs:        make([]PortableRun, 0, len(normalized.Runs)),
	}
	for _, run := range normalized.Runs {
		if run.Outcome == nil {
			return PortableSnapshot{}, fmt.Errorf("sessions: terminal run %q has no outcome", run.ID)
		}
		profile := run.ProtocolProfile
		portable.Runs = append(portable.Runs, PortableRun{
			SessionID:       run.SessionID,
			ID:              run.ID,
			SpawnedByItemID: run.SpawnedByItemID,
			Provider:        run.ModelSelection.Provider(),
			Model:           run.ModelSelection.Model(),
			Outcome:         *run.Outcome,
			Error:           run.Error,
			Metrics:         run.Metrics,
			Limits:          run.Limits,
			// Every exported run is a root today (no child edges exist to project), so
			// each states its own contract. A child would omit this and read its root's.
			ProtocolProfile: &profile,
			Detail:          run.Detail,
			CreatedAt:       run.CreatedAt,
			FinishedAt:      run.FinishedAt,
			UpdatedAt:       run.UpdatedAt,
			MessageMark:     run.MessageMark,
		})
	}
	return portable, nil
}
