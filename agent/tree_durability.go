package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrDurabilityConflict reports that an idempotency key was previously
	// committed with different content.
	ErrDurabilityConflict = errors.New("agent: durability boundary conflicts with committed content")
	// ErrTreeIncarnationConflict reports that a writer no longer owns the
	// authoritative tree head it attempted to advance.
	ErrTreeIncarnationConflict = errors.New("agent: tree incarnation conflict")
	// ErrTreeDurabilityMismatch reports an attempt to restore a durable tree in
	// an ephemeral Engine, or an ephemeral tree in a durable Engine.
	ErrTreeDurabilityMismatch = errors.New("agent: tree durability mode mismatch")
	// ErrTreeCaptureUnavailable reports that a durable tree cannot be captured
	// through the ephemeral, caller-driven checkpoint API.
	ErrTreeCaptureUnavailable = errors.New("agent: tree capture is unavailable in durable mode")
)

// EffectBoundaryKind identifies one monotonic durable transition of an
// external Effect. The zero value is invalid.
type EffectBoundaryKind string

const (
	EffectBoundaryInvalid  EffectBoundaryKind = ""
	EffectBoundaryPending  EffectBoundaryKind = "pending"
	EffectBoundarySettled  EffectBoundaryKind = "settled"
	EffectBoundaryResolved EffectBoundaryKind = "resolved"
)

func (k EffectBoundaryKind) Valid() bool {
	switch k {
	case EffectBoundaryPending, EffectBoundarySettled, EffectBoundaryResolved:
		return true
	default:
		return false
	}
}

func (k EffectBoundaryKind) String() string {
	if !k.Valid() {
		return invalidEnumName
	}
	return string(k)
}

// EffectBoundary is an immutable proposal to atomically advance one tree head
// together with one external Effect fact. Values are minted by Engine only.
type EffectBoundary struct {
	kind               EffectBoundaryKind
	request            EffectRequest
	settlement         Settlement
	hasSettlement      bool
	previousTreeDigest Digest
	treeSnapshot       TreeSnapshot
}

func newEffectBoundary(
	kind EffectBoundaryKind,
	request EffectRequest,
	settlement Settlement,
	previousTreeDigest Digest,
	treeSnapshot TreeSnapshot,
) (EffectBoundary, error) {
	hasSettlement := kind == EffectBoundarySettled || kind == EffectBoundaryResolved
	boundary := EffectBoundary{
		kind: kind, request: request, settlement: settlement,
		hasSettlement: hasSettlement, previousTreeDigest: previousTreeDigest,
		treeSnapshot: treeSnapshot,
	}
	if !boundary.Valid() {
		return EffectBoundary{}, errors.New("invalid durable Effect boundary")
	}
	return boundary, nil
}

func (b EffectBoundary) Kind() EffectBoundaryKind { return b.kind }

// Request returns the exact immutable dispatch request represented by this
// boundary.
func (b EffectBoundary) Request() EffectRequest { return b.request.clone() }

// Settlement returns the settlement introduced by a settled or resolved
// boundary. Pending boundaries return false.
func (b EffectBoundary) Settlement() (Settlement, bool) {
	return b.settlement.clone(), b.hasSettlement
}

func (b EffectBoundary) PreviousTreeDigest() Digest { return b.previousTreeDigest }

func (b EffectBoundary) TreeSnapshot() TreeSnapshot { return b.treeSnapshot }

func (b EffectBoundary) Valid() bool {
	if !b.kind.Valid() || !b.request.Valid() || !b.previousTreeDigest.Valid() ||
		!b.treeSnapshot.Valid() || b.previousTreeDigest == b.treeSnapshot.Digest() ||
		b.treeSnapshot.RootID() != b.request.Relation().RootID() {
		return false
	}
	if _, durable := b.treeSnapshot.IncarnationID(); !durable {
		return false
	}
	if !b.matchesProspectiveTree() {
		return false
	}
	switch b.kind {
	case EffectBoundaryPending:
		return !b.hasSettlement && !b.settlement.Valid()
	case EffectBoundarySettled:
		return b.hasSettlement && b.settlement.Valid() &&
			b.settlement.EffectID() == b.request.ID()
	case EffectBoundaryResolved:
		return b.hasSettlement && b.settlement.Valid() &&
			b.settlement.Status() != SettlementStatusUnknown &&
			b.settlement.EffectID() == b.request.ID()
	default:
		return false
	}
}

func (b EffectBoundary) matchesProspectiveTree() bool {
	var processSnapshot ProcessSnapshot
	for _, candidate := range b.treeSnapshot.ProcessSnapshots() {
		if candidate.ProcessID() == b.request.ProcessID() {
			processSnapshot = candidate
			break
		}
	}
	if !processSnapshot.Valid() || processSnapshot.DeploymentRef() != b.request.DeploymentRef() ||
		processSnapshot.Relation() != b.request.Relation() {
		return false
	}
	wire, err := processSnapshot.wire()
	if err != nil || wire.Prepared == nil ||
		wire.Prepared.StepSequence != b.request.StepSequence() ||
		uint64(b.request.BatchIndex()) >= uint64(len(wire.Prepared.Effects)) {
		return false
	}
	record := wire.Prepared.Effects[b.request.BatchIndex()]
	if record.ID != b.request.ID() || !sameBoundaryEffect(record.Effect, b.request.Effect()) {
		return false
	}
	if b.kind == EffectBoundaryPending {
		return record.Phase == effectPhasePending && record.Settlement == nil
	}
	return record.Phase == effectPhaseSettled && record.Settlement != nil &&
		sameBoundarySettlement(*record.Settlement, b.settlement)
}

func sameBoundaryEffect(left, right Effect) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func sameBoundarySettlement(left, right Settlement) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

// TreeCheckpointKind identifies a Runtime-owned durable program-counter cut.
// The zero value is invalid.
type TreeCheckpointKind string

const (
	TreeCheckpointInvalid  TreeCheckpointKind = ""
	TreeCheckpointParked   TreeCheckpointKind = "parked"
	TreeCheckpointTerminal TreeCheckpointKind = "terminal"
)

func (k TreeCheckpointKind) Valid() bool {
	return k == TreeCheckpointParked || k == TreeCheckpointTerminal
}

func (k TreeCheckpointKind) String() string {
	if !k.Valid() {
		return invalidEnumName
	}
	return string(k)
}

// TreeCheckpoint is an immutable Runtime-owned proposal to persist a safe
// whole-tree cut. Values are minted by Engine only.
type TreeCheckpoint struct {
	kind               TreeCheckpointKind
	previousTreeDigest Digest
	treeSnapshot       TreeSnapshot
}

func newTreeCheckpoint(
	kind TreeCheckpointKind,
	previousTreeDigest Digest,
	treeSnapshot TreeSnapshot,
) (TreeCheckpoint, error) {
	checkpoint := TreeCheckpoint{
		kind: kind, previousTreeDigest: previousTreeDigest, treeSnapshot: treeSnapshot,
	}
	if !checkpoint.Valid() {
		return TreeCheckpoint{}, errors.New("invalid durable tree checkpoint")
	}
	return checkpoint, nil
}

func (c TreeCheckpoint) Kind() TreeCheckpointKind { return c.kind }

func (c TreeCheckpoint) PreviousTreeDigest() Digest { return c.previousTreeDigest }

func (c TreeCheckpoint) TreeSnapshot() TreeSnapshot { return c.treeSnapshot }

func (c TreeCheckpoint) Valid() bool {
	if !c.kind.Valid() || !c.previousTreeDigest.Valid() || !c.treeSnapshot.Valid() ||
		c.previousTreeDigest == c.treeSnapshot.Digest() {
		return false
	}
	_, durable := c.treeSnapshot.IncarnationID()
	return durable && c.matchesSafeCut()
}

func (c TreeCheckpoint) matchesSafeCut() bool {
	allTerminal := true
	for _, snapshot := range c.treeSnapshot.ProcessSnapshots() {
		if snapshot.Status().Terminal() {
			continue
		}
		allTerminal = false
		if snapshot.Status() == StatusWaiting || snapshot.Status() == StatusPaused {
			continue
		}
		wire, err := snapshot.wire()
		if err != nil || wire.Prepared == nil {
			return false
		}
		unknown := false
		for _, effect := range wire.Prepared.Effects {
			unknown = unknown || effect.unknown()
		}
		if !unknown {
			return false
		}
	}
	return c.kind == TreeCheckpointTerminal && allTerminal ||
		c.kind == TreeCheckpointParked && !allTerminal
}

// TreeActivation transfers active-writer authority from one durable snapshot
// to a prospective snapshot with a freshly minted incarnation. Values are
// minted by Engine only.
type TreeActivation struct {
	previousIncarnationID TreeIncarnationID
	previousTreeDigest    Digest
	incarnationID         TreeIncarnationID
	treeSnapshot          TreeSnapshot
}

func newTreeActivation(
	previousIncarnationID TreeIncarnationID,
	previousTreeDigest Digest,
	incarnationID TreeIncarnationID,
	treeSnapshot TreeSnapshot,
) (TreeActivation, error) {
	activation := TreeActivation{
		previousIncarnationID: previousIncarnationID,
		previousTreeDigest:    previousTreeDigest,
		incarnationID:         incarnationID,
		treeSnapshot:          treeSnapshot,
	}
	if !activation.Valid() {
		return TreeActivation{}, errors.New("invalid durable tree activation")
	}
	return activation, nil
}

func (a TreeActivation) PreviousIncarnationID() TreeIncarnationID {
	return a.previousIncarnationID
}

func (a TreeActivation) PreviousTreeDigest() Digest { return a.previousTreeDigest }

func (a TreeActivation) IncarnationID() TreeIncarnationID { return a.incarnationID }

func (a TreeActivation) TreeSnapshot() TreeSnapshot { return a.treeSnapshot }

func (a TreeActivation) Valid() bool {
	if !a.previousIncarnationID.Valid() || !a.previousTreeDigest.Valid() ||
		!a.incarnationID.Valid() || a.previousIncarnationID == a.incarnationID ||
		!a.treeSnapshot.Valid() {
		return false
	}
	incarnationID, durable := a.treeSnapshot.IncarnationID()
	return durable && incarnationID == a.incarnationID
}

// TreeDurability is the complete Host port for active recovery. Every boundary
// that carries a prospective tree must atomically compare and advance the same
// authoritative head; root-aborted outcomes only close their admission fact.
// The implementation owns storage, transactions, product facts, deadlines,
// and ambiguous-commit reconciliation; Engine owns ordering and fencing.
type TreeDurability interface {
	ProcessStartOutcomeAcknowledger
	// ActivateTree atomically fences the previous incarnation and installs the
	// prospective snapshot before a restored tree is published.
	ActivateTree(ctx context.Context, activation TreeActivation) error
	// CommitEffect atomically records one Effect fact and advances the tree head.
	CommitEffect(ctx context.Context, boundary EffectBoundary) error
	// CommitCheckpoint atomically advances the tree head to a Runtime-owned safe cut.
	CommitCheckpoint(ctx context.Context, checkpoint TreeCheckpoint) error
}

func activateTree(
	ctx context.Context,
	durability TreeDurability,
	activation TreeActivation,
) (err error) {
	if durability == nil || !activation.Valid() {
		return errors.New("invalid durable tree activation")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tree durability activation panicked: %v", recovered)
		}
	}()
	return durability.ActivateTree(context.WithoutCancel(requireContext(ctx)), activation)
}

func commitEffectBoundary(
	ctx context.Context,
	durability TreeDurability,
	boundary EffectBoundary,
) (err error) {
	if durability == nil || !boundary.Valid() {
		return errors.New("invalid durable Effect boundary")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tree durability Effect commit panicked: %v", recovered)
		}
	}()
	return durability.CommitEffect(context.WithoutCancel(requireContext(ctx)), boundary)
}

func commitTreeCheckpoint(
	ctx context.Context,
	durability TreeDurability,
	checkpoint TreeCheckpoint,
) (err error) {
	if durability == nil || !checkpoint.Valid() {
		return errors.New("invalid durable tree checkpoint")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tree durability checkpoint commit panicked: %v", recovered)
		}
	}()
	return durability.CommitCheckpoint(
		context.WithoutCancel(requireContext(ctx)), checkpoint,
	)
}
