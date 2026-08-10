package sessions

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ErrInvalidPortableSnapshot marks a structurally decoded archive that cannot
// satisfy the product aggregate invariants.
var ErrInvalidPortableSnapshot = errors.New("sessions: invalid portable snapshot")

// PortableSnapshot is the transport-neutral, terminal-only session archive
// accepted by restore. It deliberately separates a portable run's outcome from
// its derived lifecycle state: restore owns rebuilding that state machine.
type PortableSnapshot struct {
	Session     PortableSession
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []PortableRun
	ToolResults []toolresult.Blob
	// Plan is the session's Plan, carried as a value so an archive restores
	// the work plan attached to the conversation rather than just the conversation.
	Plan []plan.Step
}

// PortableSession is the terminal archive identity. It intentionally excludes
// live aggregate details such as lineage, isolation, and revision: an
// imported archive is always admitted as a standalone conversation.
type PortableSession struct {
	ID        string
	Title     string
	CWD       string
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
	// ParentRunID and RootRunID are the child edges. Together with
	// SpawnedByItemID they preserve the execution tree exactly across export/import.
	ParentRunID string
	RootRunID   string
	Provider    string
	Model       string
	Outcome     run.Outcome
	Failure     *run.Failure
	Metrics     run.Metrics
	Limits      run.Limits
	// Capabilities is a pointer because an empty set is a known minimal Run while
	// nil means the archive omitted the root-owned fact. A root must carry it; a
	// child must not and inherits its root's value.
	Capabilities *run.Capabilities
	Detail       string
	CreatedAt    time.Time
	FinishedAt   time.Time
	UpdatedAt    time.Time
	MessageMark  int
}

// rootID is the Run that owns this Run's capabilities: itself for a root and
// RootRunID for a child.
func (p PortableRun) rootID() string {
	if p.RootRunID != "" {
		return p.RootRunID
	}
	return p.ID
}

// validateLineage checks the identity rules a schema cannot: a root carries its
// capabilities, a child carries none of its own, and a
// child's edges name other runs in the same session rather than itself.
//
// These are not shape rules — JSON Schema cannot compare two fields, and "root"
// is the ABSENCE of the child edges, which no presence rule can condition on — so
// they belong to the transaction that turns an archive into a session.
func (p PortableRun) validateLineage() error {
	lineage := run.Lineage{
		SpawnedByItemID: p.SpawnedByItemID,
		ParentRunID:     p.ParentRunID,
		RootRunID:       p.RootRunID,
	}
	if err := lineage.Validate(p.ID); err != nil {
		return err
	}
	if lineage.IsRoot() {
		if p.Capabilities == nil {
			return fmt.Errorf("root run %q carries no capabilities", p.ID)
		}
		return nil
	}
	if p.Capabilities != nil {
		return fmt.Errorf("child run %q carries capabilities of its own", p.ID)
	}
	return nil
}

// CanonicalSnapshot rebuilds and validates the canonical aggregate from a
// portable archive. The restore use case owns this normalization.
func (p PortableSnapshot) CanonicalSnapshot() (Snapshot, error) {
	snapshot := Snapshot{
		Session:     p.Session.session(),
		Messages:    p.Messages,
		Items:       append([]transcript.Item(nil), p.Items...),
		ToolResults: append([]toolresult.Blob(nil), p.ToolResults...),
		Runs:        make([]run.Run, 0, len(p.Runs)),
		Plan:        append([]plan.Step(nil), p.Plan...),
	}
	if err := plan.Validate(snapshot.Plan); err != nil {
		return Snapshot{}, fmt.Errorf("%w: plan: %w", ErrInvalidPortableSnapshot, err)
	}
	// A child reads its root's capabilities, so root values are collected before
	// any Run is rebuilt. The archive states each value exactly once.
	capabilitySets := make(map[string]run.Capabilities, len(p.Runs))
	for _, portable := range p.Runs {
		if portable.Capabilities != nil {
			capabilitySets[portable.ID] = *portable.Capabilities
		}
	}
	for _, portable := range p.Runs {
		if err := portable.validateLineage(); err != nil {
			return Snapshot{}, fmt.Errorf("%w: %w", ErrInvalidPortableSnapshot, err)
		}
		if _, known := capabilitySets[portable.rootID()]; !known {
			return Snapshot{}, fmt.Errorf("%w: run %q names root %q, which this archive does not contain",
				ErrInvalidPortableSnapshot, portable.ID, portable.rootID())
		}
		selection, err := modelref.New(portable.Provider, portable.Model)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: run %q model selection: %w", ErrInvalidPortableSnapshot, portable.ID, err)
		}
		state, ok := run.Running.Terminate(portable.Outcome)
		if !ok {
			return Snapshot{}, fmt.Errorf("%w: run %q has invalid outcome %s", ErrInvalidPortableSnapshot, portable.ID, portable.Outcome)
		}
		outcome := portable.Outcome
		restored, err := run.Restore(run.Snapshot{
			SessionID: portable.SessionID,
			ID:        portable.ID,
			Lineage: run.Lineage{
				SpawnedByItemID: portable.SpawnedByItemID,
				ParentRunID:     portable.ParentRunID,
				RootRunID:       portable.RootRunID,
			},
			ModelSelection: selection,
			State:          state,
			Outcome:        &outcome,
			Failure:        portable.Failure,
			Metrics:        portable.Metrics,
			Limits:         portable.Limits,
			Capabilities:   capabilitySets[portable.rootID()],
			Detail:         portable.Detail,
			CreatedAt:      portable.CreatedAt,
			FinishedAt:     portable.FinishedAt,
			UpdatedAt:      portable.UpdatedAt,
			MessageMark:    portable.MessageMark,
		})
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: run %q: %w", ErrInvalidPortableSnapshot, portable.ID, err)
		}
		snapshot.Runs = append(snapshot.Runs, restored)
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
		CWD:       p.CWD,
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
		invocation.Offload = &toolresult.Ref{ID: blob.ID}
		item.Tool = &invocation
	}
	return nil
}

// PortableSnapshot returns the normalized, terminal-only representation used by
// archive encoders.
func (snapshot Snapshot) PortableSnapshot() (PortableSnapshot, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return PortableSnapshot{}, err
	}
	portable := PortableSnapshot{
		Session: PortableSession{
			ID:        normalized.Session.ID,
			Title:     normalized.Session.Title,
			CWD:       normalized.Session.CWD,
			Model:     normalized.Session.Model,
			CreatedAt: normalized.Session.StartedAt,
			UpdatedAt: normalized.Session.UpdatedAt,
			Favorite:  normalized.Session.Favorite,
		},
		Messages:    normalized.Messages,
		Items:       normalized.Items,
		ToolResults: normalized.ToolResults,
		Plan:        normalized.Plan,
		Runs:        make([]PortableRun, 0, len(normalized.Runs)),
	}
	for _, run := range normalized.Runs {
		outcome, terminal := run.Outcome()
		if !terminal {
			return PortableSnapshot{}, fmt.Errorf("sessions: terminal run %q has no outcome", run.ID())
		}
		failure, failed := run.Failure()
		portableRun := PortableRun{
			SessionID:       run.SessionID(),
			ID:              run.ID(),
			SpawnedByItemID: run.Lineage().SpawnedByItemID,
			ParentRunID:     run.Lineage().ParentRunID,
			RootRunID:       run.Lineage().RootRunID,
			Provider:        run.ModelSelection().Provider(),
			Model:           run.ModelSelection().Model(),
			Outcome:         outcome,
			Metrics:         run.Metrics(),
			Limits:          run.Limits(),
			Detail:          run.Detail(),
			CreatedAt:       run.CreatedAt(),
			FinishedAt:      run.FinishedAt(),
			UpdatedAt:       run.UpdatedAt(),
			MessageMark:     run.MessageMark(),
		}
		if failed {
			portableRun.Failure = &failure
		}
		if run.Lineage().IsRoot() {
			capabilities := run.Capabilities()
			portableRun.Capabilities = &capabilities
		}
		portable.Runs = append(portable.Runs, portableRun)
	}
	return portable, nil
}
